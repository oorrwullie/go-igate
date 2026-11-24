package restartmanager

import (
 "errors"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
)

func TestRestartPlansAcrossPlatforms(t *testing.T){
 helper:=filepath.Join(t.TempDir(),"sparky-restart")
 if err:=os.WriteFile(helper,[]byte("#!/bin/sh\nexit 0\n"),0700);err!=nil{t.Fatal(err)}
 exists:=func(s string)bool{return s=="/run/systemd/system"}
 find:=func(name string)(string,error){return "/usr/bin/"+name,nil}
 cases:=[]struct{os,manager,target,helper,wantManager,wantBin string;wantArgs []string}{
  {"linux","auto","","","systemd","/usr/bin/systemctl",[]string{"restart","sparky.service"}},
  {"linux","openrc","","","openrc","/usr/bin/rc-service",[]string{"sparky","restart"}},
  {"linux","sysv","","","sysv","/usr/bin/service",[]string{"sparky","restart"}},
  {"linux","runit","","","runit","/usr/bin/sv",[]string{"restart","sparky"}},
  {"linux","s6","","","s6","/usr/bin/s6-svc",[]string{"-r","/run/service/sparky"}},
  {"darwin","auto","","","launchd","/usr/bin/launchctl",[]string{"kickstart","-k","system/org.sparky.gateway"}},
  {"darwin","launchd","gui/501/org.sparky.gateway","","launchd","/usr/bin/launchctl",[]string{"kickstart","-k","gui/501/org.sparky.gateway"}},
  {"linux","custom","",helper,"custom",helper,nil},
  {"darwin","custom","",helper,"custom",helper,nil},
 }
 for _,tc:=range cases{
  t.Run(tc.os+"/"+tc.manager+"/"+tc.target,func(t *testing.T){
   p,err:=resolve(tc.os,tc.manager,tc.target,tc.helper,exists,find)
   if err!=nil{t.Fatal(err)}
   if p.Manager!=tc.wantManager||p.Executable!=tc.wantBin||!reflect.DeepEqual(p.Args,tc.wantArgs){
    t.Fatalf("got %+v, want %s %s %v",p,tc.wantManager,tc.wantBin,tc.wantArgs)
   }
  })
 }
}
func TestRejectUnsafeOrUnsupportedPlans(t *testing.T){
 noSystemd:=func(string)bool{return false}
 find:=func(s string)(string,error){return "/usr/bin/"+s,nil}
 cases:=[]struct{os,manager,target,helper string}{
  {"linux","auto","",""},
  {"windows","systemd","",""},
  {"linux","launchd","",""},
  {"darwin","systemd","",""},
  {"linux","docker","",""},
  {"linux","custom","",""},
  {"linux","custom","sparky","/some/command"},
  {"linux","custom","","relative/helper"},
  {"linux","systemd","","/some/helper"},
  {"linux","systemd","-evil",""},
  {"linux","openrc","sparky;rm -rf /",""},
  {"linux","systemd","foo\nbar",""},
  {"darwin","launchd","other/service",""},
  {"darwin","launchd","gui/abc/org.sparky.gateway",""},
 }
 for _,tc:=range cases{
  if p,err:=resolve(tc.os,tc.manager,tc.target,tc.helper,noSystemd,find);err==nil{
   t.Errorf("accepted %q/%q/%q/%q as %+v",tc.os,tc.manager,tc.target,tc.helper,p)
  }
 }
}
func TestMissingSupervisorExecutableFailsClosed(t *testing.T){
 noBinary:=func(string)(string,error){return "",errors.New("missing")}
 _,err:=resolve("linux","openrc","","",func(string)bool{return false},noBinary)
 if err==nil || !strings.Contains(err.Error(),"unavailable"){t.Fatalf("missing binary not rejected: %v",err)}
}

func TestAutoManagerPrefersKubernetesOverHostInit(t *testing.T){
 for _,tc:=range []struct{goos string;k8s bool;want string}{
  {"linux",true,"kubernetes"},
  {"linux",false,"auto"},
  {"darwin",false,"launchd"},
  {"darwin",true,"launchd"},
  {"windows",false,"auto"},
 }{
  if got:=detectManager(tc.goos,tc.k8s);got!=tc.want{
   t.Errorf("detectManager(%q,%t)=%q want %q",tc.goos,tc.k8s,got,tc.want)
  }
 }
}

func TestCustomHelperValidation(t *testing.T) {
 dir:=t.TempDir()
 executable:=filepath.Join(dir,"restart-helper")
 notExecutable:=filepath.Join(dir,"plain-file")
 if err:=os.WriteFile(executable,[]byte("#!/bin/sh\nexit 0\n"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(notExecutable,[]byte("not an executable\n"),0600);err!=nil{t.Fatal(err)}
 noSystemd:=func(string)bool{return false}
 find:=func(name string)(string,error){return "/usr/bin/"+name,nil}
 for _,tc:=range []struct{name,path string;valid bool}{
  {"executable file",executable,true},
  {"missing path",filepath.Join(dir,"missing"),false},
  {"directory",dir,false},
  {"non-executable file",notExecutable,false},
 }{
  t.Run(tc.name,func(t *testing.T){
   p,err:=resolve("linux","custom","",tc.path,noSystemd,find)
   if tc.valid && (err!=nil || p.Executable!=tc.path){t.Fatalf("valid helper: plan=%+v err=%v",p,err)}
   if !tc.valid && err==nil{t.Fatalf("invalid helper accepted: %+v",p)}
  })
 }
}
