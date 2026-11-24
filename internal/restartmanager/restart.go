// Package restartmanager defines fixed, operator-selected service restart plans.
// No HTTP request data is ever accepted as a command, target, or argument.
package restartmanager

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
)

type Plan struct {
 Manager string
 Executable string
 Args []string
 kubernetes *KubernetesConfig
}

// Resolve chooses a fixed restart plan at process startup. "auto" is
// conservative: only systemd or launchd is automatically detected.
// Other Linux supervisors must be explicitly selected by the operator.
func Resolve(manager, target, helper string) (Plan,error) {
 // Kubernetes pods frequently have no host init system visible. Prefer
 // Kubernetes only with explicit in-cluster evidence, not /proc/1 guesses.
 if manager=="" || manager=="auto" {
  if runtime.GOOS=="linux" && os.Getenv("KUBERNETES_SERVICE_HOST")!="" {
   manager="kubernetes"
  }
 }
 if manager=="kubernetes" {
  if runtime.GOOS!="linux" {return Plan{},errors.New("Kubernetes restart requires Linux")}
  if helper!="" {return Plan{},errors.New("Kubernetes restart does not accept a helper")}
  return kubernetesPlan(target)
 }
 return resolve(runtime.GOOS,manager,target,helper,func(p string) bool {
  st,err:=os.Stat(p)
  return err==nil && st.IsDir()
 },findBinary)
}

func resolve(goos,manager,target,helper string,isDir func(string)bool,find func(string)(string,error))(Plan,error){
 if manager=="" || manager=="auto" {
  switch {
  case goos=="darwin": manager="launchd"
  case goos=="linux" && isDir("/run/systemd/system"): manager="systemd"
  default: return Plan{},errors.New("unable to auto-detect service manager; select --restart-manager explicitly")
  }
 }
 if goos!="linux" && goos!="darwin" {return Plan{},errors.New("service restart is supported only on Linux and macOS")}
 if helper!="" && manager!="custom" {return Plan{},errors.New("restart helper may only be set with the custom manager")}
 if manager=="custom" {
  if !filepath.IsAbs(helper) || helper=="/" {return Plan{},errors.New("custom restart helper must be an absolute executable path")}
  if target!="" {return Plan{},errors.New("custom restart helper does not accept a service target")}
  return Plan{Manager:"custom",Executable:helper},nil
 }
 if !validTarget(target) {return Plan{},errors.New("invalid restart service target")}
 // Only launchd uses a domain/label path. Linux manager targets are fixed
 // service names, never paths or traversal into another service directory.
 if manager!="launchd" && strings.Contains(target,"/") {
  return Plan{},errors.New("Linux restart target must be a service name, not a path")
 }
 var binary string
 var args []string
 switch manager {
 case "systemd":
  if goos!="linux"{return Plan{},errors.New("systemd requires Linux")}
  if target==""{target="sparky.service"}
  binary="systemctl";args=[]string{"restart",target}
 case "openrc":
  if goos!="linux"{return Plan{},errors.New("OpenRC requires Linux")}
  if target==""{target="sparky"}
  binary="rc-service";args=[]string{target,"restart"}
 case "sysv":
  if goos!="linux"{return Plan{},errors.New("SysV requires Linux")}
  if target==""{target="sparky"}
  binary="service";args=[]string{target,"restart"}
 case "runit":
  if goos!="linux"{return Plan{},errors.New("runit requires Linux")}
  if target==""{target="sparky"}
  binary="sv";args=[]string{"restart",target}
 case "s6":
  if goos!="linux"{return Plan{},errors.New("s6 requires Linux")}
  if target==""{target="sparky"}
  binary="s6-svc";args=[]string{"-r","/run/service/"+target}
 case "launchd":
  if goos!="darwin"{return Plan{},errors.New("launchd requires macOS")}
  if target==""{target="system/org.sparky.gateway"}
  if !validLaunchdTarget(target){return Plan{},errors.New("launchd target must use system/<label> or gui/<uid>/<label>")}
  binary="launchctl";args=[]string{"kickstart","-k",target}
 default:
  return Plan{},fmt.Errorf("unsupported restart manager %q",manager)
 }
 exe,err:=find(binary)
 if err!=nil{return Plan{},fmt.Errorf("restart manager %s unavailable: %w",manager,err)}
 return Plan{Manager:manager,Executable:exe,Args:args},nil
}

func validTarget(target string)bool{
 if target==""{return true}
 if len(target)>160 || target[0]=='-' || strings.Contains(target,".."){return false}
 for _,r:=range target{
  if (r>='a'&&r<='z')||(r>='A'&&r<='Z')||(r>='0'&&r<='9')||r=='_'||r=='-'||r=='.'||r=='/' {continue}
  return false
 }
 return true
}
func validLaunchdTarget(target string)bool{
 parts:=strings.Split(target,"/")
 if len(parts)==2 && parts[0]=="system"{return parts[1]!="" && parts[1][0]!='-'}
 if len(parts)==3 && parts[0]=="gui"{
  if len(parts[1])==0 || parts[2]=="" || parts[2][0]=='-' {return false}
  for _,r:=range parts[1]{if r<'0'||r>'9'{return false}}
  return true
 }
 return false
}

func findBinary(name string)(string,error){
 for _,dir:=range []string{"/usr/bin","/bin","/usr/sbin","/sbin","/usr/local/bin","/opt/homebrew/bin"}{
  path:=filepath.Join(dir,name)
  info,err:=os.Stat(path)
  if err==nil && info.Mode().IsRegular() && info.Mode()&0111!=0{return path,nil}
 }
 return "",errors.New("binary not present in standard system directories")
}

// Run never invokes a shell. The selected plan is immutable once installed
// in the server, and neither browser input nor config YAML changes its target.
func (p Plan) Run(ctx context.Context)error{
 if p.kubernetes!=nil {return p.kubernetes.Restart(ctx)}
 if p.Executable=="" {return errors.New("restart adapter not configured")}
 return exec.CommandContext(ctx,p.Executable,p.Args...).Run()
}
