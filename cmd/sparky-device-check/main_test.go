package main

import (
 "os"
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
 if len(r.errors)!=1||!strings.Contains(r.errors[0],"RTL-SDR receiver enabled"){t.Fatalf("unexpected errors: %+v",r)}
 if len(r.warnings)==0||!strings.Contains(strings.Join(r.warnings," "),"rtl_test"){t.Fatalf("missing corrective guidance: %+v",r)}
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
