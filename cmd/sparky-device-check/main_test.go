package main

import (
 "os"
 "fmt"
 "strconv"
 "path/filepath"
 "strings"
 "testing"

 "github.com/oorrwullie/sparky/internal/config"
)

func deviceConfig(t *testing.T, yaml string) config.Config {
 t.Helper()
 cfg,err:=config.ParseConfigBytes([]byte(yaml))
 if err!=nil{t.Fatal(err)}
 return cfg
}
func TestPreflightMQTTNeedsNoLocalDevices(t *testing.T){
 root:=t.TempDir()
 c:=deviceConfig(t,"openwebrx-mqtt:\n  enabled: true\nsdr:\n  enabled: true\ntransmitter:\n  enabled: false\n")
 r:=checkDevices(c,root,root)
 if len(r.errors)!=0||len(r.warnings)!=0{t.Fatalf("MQTT-only mode falsely requires local devices: %+v",r)}
}
func TestPreflightMissingSDRIsActionable(t *testing.T){
 root:=t.TempDir()
 c:=deviceConfig(t,"sdr:\n  enabled: true\n")
 r:=checkDevices(c,root,root)
 if len(r.errors)!=0 || len(r.warnings)==0 || !strings.Contains(strings.Join(r.warnings," "),"rtl_test"){t.Fatalf("unknown hardware should be advisory with guidance: %+v",r)}
}
func TestPreflightSoundcardFallback(t *testing.T){
 root:=t.TempDir()
 c:=deviceConfig(t,"sdr:\n  enabled: false\n")
 r:=checkDevices(c,root,root)
 if len(r.errors)==0||!strings.Contains(r.errors[0],"Audio backend enabled"){t.Fatalf("soundcard fallback undetected: %+v",r)}
}
func TestPreflightSoundcardOverrideSkipsSDR(t *testing.T){
 root:=t.TempDir()
 c:=deviceConfig(t,"sdr:\n  enabled: true\nsoundcard-capture-enabled: true\n")
 r:=checkDevices(c,root,root)
 if len(r.errors)==0||strings.Contains(strings.Join(r.errors," "),"RTL-SDR"){t.Fatalf("unexpected backend selection: %+v",r)}
}
func TestRTLSysfsDiscoveryUsesRealBusAndDeviceNumbers(t *testing.T){
 root:=t.TempDir()
 dir:=filepath.Join(root,"sys","bus","usb","devices","1-1")
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 for name,data:=range map[string]string{"idVendor":"0bda\n","idProduct":"2838\n","busnum":"1\n","devnum":"12\n"}{
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(data),0600);err!=nil{t.Fatal(err)}
 }
 nodes,err:=findRTLNodes(filepath.Join(root,"sys"),filepath.Join(root,"dev"))
 if err!=nil||len(nodes)!=1||nodes[0]!=filepath.Join(root,"dev","bus","usb","001","012"){
  t.Fatalf("unexpected RTL discovery: %v %v",nodes,err)
 }
 // A different USB VID is not misidentified as an RTL receiver.
 if err:=os.WriteFile(filepath.Join(dir,"idVendor"),[]byte("0001"),0600);err!=nil{t.Fatal(err)}
 nodes,err=findRTLNodes(filepath.Join(root,"sys"),filepath.Join(root,"dev"))
 if err!=nil||len(nodes)!=0{t.Fatalf("non-RTL accepted: %v %v",nodes,err)}
}
func TestPreflightRefusesOrdinaryFileMasqueradingAsDevice(t *testing.T){
 root:=t.TempDir()
 node:=filepath.Join(root,"fake")
 if err:=os.WriteFile(node,[]byte("not a device"),0600);err!=nil{t.Fatal(err)}
 r:=result{}
 checkedNode(&r,node,"RTL-SDR")
 if len(r.errors)!=1||!strings.Contains(r.errors[0],"not a device node"){t.Fatalf("accepted fake node: %+v",r)}
}

func fakeRTLTuner(t *testing.T,root,entry,serial,bus,device string,allowed bool){
 t.Helper()
 dir:=filepath.Join(root,"sys","bus","usb","devices",entry)
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 for name,data:=range map[string]string{"idVendor":"0bda\n","idProduct":"2838\n","busnum":bus+"\n","devnum":device+"\n","serial":serial+"\n"} {
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(data),0600);err!=nil{t.Fatal(err)}
 }
 usb:=filepath.Join(root,"dev","bus","usb")
 if err:=os.MkdirAll(usb,0700);err!=nil{t.Fatal(err)}
 node:=filepath.Join(usb,fmt.Sprintf("%03d",mustNum(t,bus)),fmt.Sprintf("%03d",mustNum(t,device)))
 if err:=os.MkdirAll(filepath.Dir(node),0700);err!=nil{t.Fatal(err)}
 if allowed {
  if err:=os.Symlink("/dev/null",node);err!=nil{t.Fatal(err)}
 }else{
  if err:=os.WriteFile(node,[]byte("restricted test fixture"),0600);err!=nil{t.Fatal(err)}
 }
}
func TestSelectedRTLSerialIgnoresOtherRestrictedDongle(t *testing.T){
 root:=t.TempDir()
 fakeRTLTuner(t,root,"1-1","APRS", "1","1",true)
 fakeRTLTuner(t,root,"1-2","ADSB", "1","2",false)
 c:=deviceConfig(t,"sdr:\n  enabled: true\n  device: APRS\n")
 r:=checkDevices(c,filepath.Join(root,"sys"),filepath.Join(root,"dev"))
 if len(r.errors)!=0||len(r.warnings)!=0{t.Fatalf("selected accessible SDR rejected: %+v",r)}
 c.Sdr.Device="ADSB"
 r=checkDevices(c,filepath.Join(root,"sys"),filepath.Join(root,"dev"))
 if len(r.errors)!=1 || !strings.Contains(r.errors[0],"not a device node") {
  t.Fatalf("restricted selected SDR not checked: %+v",r)
 }
}
func TestAmbiguousRTLIndexAndUnknownSerialAreAdvisory(t *testing.T){
 root:=t.TempDir()
 fakeRTLTuner(t,root,"1-1","APRS","1","1",true)
 fakeRTLTuner(t,root,"1-2","ADSB","1","2",false)
 for _,selector:=range []string{"0","1","MISSING"}{
  c:=deviceConfig(t,"sdr:\n  enabled: true\n  device: "+selector+"\n")
  r:=checkDevices(c,filepath.Join(root,"sys"),filepath.Join(root,"dev"))
  if len(r.errors)!=0||len(r.warnings)==0||!strings.Contains(strings.Join(r.warnings," "),"unverified"){
   t.Fatalf("selector %q must report inconclusive discovery, not test unrelated device: %+v",selector,r)
  }
 }
}

func mustNum(t *testing.T,s string)int{
 t.Helper()
 n,err:=strconv.Atoi(s)
 if err!=nil{t.Fatal(err)}
 return n
}

func TestSelectedRTLNumericSerialFallback(t *testing.T) {
 root := t.TempDir()
 fakeRTLTuner(t, root, "1-1", "12345678", "1", "1", true)
 fakeRTLTuner(t, root, "1-2", "99877885", "1", "2", false)
 c := deviceConfig(t, "sdr:\n  enabled: true\n  device: \"99877885\"\n")
 r := checkDevices(c, filepath.Join(root, "sys"), filepath.Join(root, "dev"))
 if len(r.errors)!=1 || !strings.Contains(r.errors[0], "not a device node") { t.Fatalf("selected numeric serial not checked: %+v", r) }
 c.Sdr.Device = "12345678"
 r = checkDevices(c, filepath.Join(root, "sys"), filepath.Join(root, "dev"))
 if len(r.errors)!=0 || len(r.warnings)!=0 { t.Fatalf("accessible numeric serial rejected: %+v", r) }
}
