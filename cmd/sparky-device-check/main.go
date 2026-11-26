// sparky-device-check validates basic access to the devices selected in
// config.yml for the non-root account running the installer. It does not
// configure privileges and cannot prove that a radio/driver will work.
package main

import (
 "flag"
 "fmt"
 "os"
 "os/user"
 "syscall"

 "golang.org/x/sys/unix"
 "path/filepath"
 "strconv"
 "strings"
 
 "github.com/oorrwullie/sparky/internal/config"
)

type device struct {
 path string
 kind string
 group string
 read bool
 write bool
}
type result struct {
 errors []string
 warnings []string
}

func (r *result) problem(format string, args ...any) {
 r.errors=append(r.errors,fmt.Sprintf(format,args...))
}
func (r *result) warn(format string,args ...any) {
 r.warnings=append(r.warnings,fmt.Sprintf(format,args...))
}
func access(path string,read,write bool) bool {
 mode:=uint32(0)
 if read {mode|=4}
 if write {mode|=2}
 return syscallAccess(path,mode)==nil
}
// Testable seam for Linux's real-UID and supplementary-group access check.
// The executable is intended to run as the systemd User= account.
var syscallAccess = func(path string, mode uint32)error {
 return unixAccess(path,mode)
}

func checkedNode(r *result, path, kind string) {
 info,err:=os.Stat(path)
 if err!=nil {
  r.problem("%s device %s: %v",kind,path,err)
  return
 }
 if info.Mode()&os.ModeDevice==0 {
  r.problem("%s path %s is not a device node",kind,path)
  return
 }
 if access(path,true,true){return}
 group:="(unknown)"
 if g:=fileGroup(path);g!=""{group=g}
 r.problem("%s device %s is not readable and writable by this user (owner/group: %s)",kind,path,group)
 r.warn("For %s, inspect: ls -l %s; id -nG; getfacl %s (if installed)",kind,path,path)
 if group=="dialout"||group=="audio"||group=="video"||group=="plugdev" {
  r.warn("If your trusted device policy uses group %s, an administrator may grant that specific device group with: sudo usermod -aG %s %s; then fully log out and log back in before retrying. Do not run Sparky as root.",group,group,os.Getenv("USER"))
 }
}
func fileGroup(path string) string {
 info,err:=os.Stat(path)
 if err!=nil{return ""}
 stat,ok:=info.Sys().(*syscall.Stat_t)
 if !ok{return ""}
 group,err:=user.LookupGroupId(strconv.FormatUint(uint64(stat.Gid),10))
 if err!=nil{return ""}
 return group.Name
}
func unixAccess(path string,mode uint32)error{return unix.Access(path,mode)}

type rtlDevice struct {
 path string
 serial string
}

// Sysfs enumeration is not guaranteed to use the same index ordering as
// librtlsdr. Only a unique device or exact serial match can be verified.
func findRTLDevices(sysroot,devroot string)([]rtlDevice,error){
 paths,err:=filepath.Glob(filepath.Join(sysroot,"bus/usb/devices/*/idVendor"))
 if err!=nil{return nil,err}
 var found []rtlDevice
 for _,p:=range paths {
  vendor,err:=os.ReadFile(p)
  if err!=nil||strings.ToLower(strings.TrimSpace(string(vendor)))!="0bda"{continue}
  product,err:=os.ReadFile(filepath.Join(filepath.Dir(p),"idProduct"))
  if err!=nil{continue}
  switch strings.ToLower(strings.TrimSpace(string(product))){
  case "2832","2838":
  default:continue
  }
  bus,err1:=os.ReadFile(filepath.Join(filepath.Dir(p),"busnum"))
  index,err2:=os.ReadFile(filepath.Join(filepath.Dir(p),"devnum"))
  if err1!=nil||err2!=nil{continue}
  b,e1:=strconv.Atoi(strings.TrimSpace(string(bus)))
  d,e2:=strconv.Atoi(strings.TrimSpace(string(index)))
  if e1!=nil||e2!=nil{continue}
  serial,_:=os.ReadFile(filepath.Join(filepath.Dir(p),"serial"))
  found=append(found,rtlDevice{
   path:filepath.Join(devroot,"bus","usb",fmt.Sprintf("%03d",b),fmt.Sprintf("%03d",d)),
   serial:strings.TrimSpace(string(serial)),
  })
 }
 return found,nil
}
func findRTLNodes(sysroot,devroot string)([]string,error){
 devices,err:=findRTLDevices(sysroot,devroot)
 if err!=nil{return nil,err}
 nodes:=make([]string,0,len(devices))
 for _,device:=range devices{nodes=append(nodes,device.path)}
 return nodes,nil
}
func checkSelectedRTL(r *result,selector string,devices []rtlDevice){
 selector=strings.TrimSpace(selector)
 if len(devices)==0 {
  r.warn("RTL-SDR requested but no known Realtek 0bda:2832/2838 USB devices found. Verify the configured sdr.device using lsusb and rtl_test -d <selector>; USB identity and permissions were not verified.")
  return
 }
 // Device indices are assigned by librtlsdr, not sysfs. A unique physical
 // tuner is unambiguous for index zero; multiple tuners are advisory.
 index,err:=strconv.Atoi(selector)
 if selector=="" {index=0;err=nil}
 if err==nil && index>=0 && index<len(devices) {
  if index==0 && len(devices)==1 {
   checkedNode(r,devices[0].path,"selected RTL-SDR")
   return
  }
  r.warn("RTL-SDR device index %q cannot be mapped reliably to a specific USB node among %d discovered tuners; access for the selected dongle is unverified. Run rtl_test -d %s as the service account and inspect its /dev/bus/usb node.",selector,len(devices),selector)
  return
 }
 var matches []rtlDevice
 for _,dev:=range devices {if dev.serial==selector {matches=append(matches,dev)}}
 if len(matches)==1{
  checkedNode(r,matches[0].path,"selected RTL-SDR")
  return
 }
 r.warn("RTL-SDR serial %q matched %d supported USB devices; selected device access is unverified. Check rtl_test -d %s, lsusb, and device serials as the service account.",selector,len(matches),selector)
}

func checkDevices(cfg config.Config,sysroot,devroot string)result{
 var r result
 // MQTT reception bypasses the local RTL-SDR/sound-card receive backend.
 if !cfg.OpenWebRXMQTT.Enabled && !cfg.SoundcardCapture && cfg.Sdr.Enabled {
  devices,err:=findRTLDevices(sysroot,devroot)
  if err!=nil{
   r.warn("RTL-SDR USB enumeration failed: %v; verify selected device manually with rtl_test",err)
  }else {
   checkSelectedRTL(&r,cfg.Sdr.Device,devices)
  }
 }
 if cfg.Transmitter.Enabled || (!cfg.OpenWebRXMQTT.Enabled && (cfg.SoundcardCapture || !cfg.Sdr.Enabled)) {
  // Only verify ACL/mode on the selected audio device(s) when those nodes
  // are known. Default PortAudio device selection needs host-level testing.
  r.warn("Audio capture/transmit is enabled. PortAudio device selection and ALSA/logind ACLs cannot be proven from config alone. Verify with the service account using arecord -l / aplay -l and a real receive/TX test; service accounts may not inherit interactive-session ACLs.")
  nodes,_:=filepath.Glob(filepath.Join(devroot,"snd","pcm*"))
  if len(nodes)==0 {
   r.problem("Audio backend enabled but no ALSA PCM device node is present under %s",filepath.Join(devroot,"snd"))
  }else {
   usable:=false
   for _,node:=range nodes {
    if access(node,true,true){usable=true;break}
   }
   if !usable {
    r.problem("Audio backend enabled but this user cannot read/write any ALSA PCM device node")
    r.warn("Inspect ls -l /dev/snd/pcm* and id -nG; configure the device's existing ACL/group for the *non-root* service account, then re-login and retry.")
   }
  }
 }
 return r
}
func main(){
 configPath:=flag.String("config","","Sparky YAML to inspect")
 flag.Parse()
 if *configPath==""{fmt.Fprintln(os.Stderr,"--config is required");os.Exit(2)}
 bytes,err:=os.ReadFile(*configPath)
 if err!=nil{fmt.Fprintln(os.Stderr,"preflight: cannot read config:",err);os.Exit(2)}
 cfg,err:=config.ParseConfigBytes(bytes)
 if err!=nil{fmt.Fprintln(os.Stderr,"preflight: invalid config (inspect locally):",err);os.Exit(2)}
 r:=checkDevices(cfg,"/sys","/dev")
 for _,w:=range r.warnings{fmt.Fprintln(os.Stderr,"NOTICE:",w)}
 if len(r.errors)>0{
  for _,e:=range r.errors{fmt.Fprintln(os.Stderr,"BLOCKED:",e)}
  fmt.Fprintln(os.Stderr,"Installation stopped before sudo/systemd changes. Correct permissions or config and rerun; this installer never auto-changes hardware group membership.")
  os.Exit(1)
 }
 fmt.Println("Device preflight passed for discoverable devices; verify the actual driver/hardware after service startup.")
}
