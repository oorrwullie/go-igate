// sparky-config serves the authenticated Sparky configuration UI.
package main

import (
 "flag"
 "log"

 "github.com/oorrwullie/sparky/internal/configeditor"
 "github.com/oorrwullie/sparky/internal/restartmanager"
 "github.com/oorrwullie/sparky/internal/webconfig"
)

func main() {
 addr:=flag.String("listen","127.0.0.1:8080","listen IP:port (non-loopback requires HTTPS)")
 path:=flag.String("config","config.yml","path to Sparky configuration")
 cert:=flag.String("tls-cert","","TLS certificate PEM path (required for non-loopback)")
 key:=flag.String("tls-key","","TLS private key PEM path (required for non-loopback)")
 publicOrigin:=flag.String("public-origin","","browser-visible HTTPS origin when Routy terminates TLS")
 proxyIP:=flag.String("trusted-proxy-ip","","Routy source IP (required for non-loopback HTTP proxying)")
 allowLANHTTP:=flag.Bool("allow-lan-http",false,"explicitly permit unencrypted Routy-to-Sparky LAN hop")
 enableRestart:=flag.Bool("enable-restart",false,"opt in to restarting only the operator-selected Sparky service (requires OS permission)")
 restartManager:=flag.String("restart-manager","auto","restart supervisor: auto, systemd, openrc, sysv, runit, s6, launchd, custom")
 restartTarget:=flag.String("restart-target","","fixed service/launchd target; empty uses the manager-specific Sparky default")
 restartHelper:=flag.String("restart-helper","","absolute path to a fixed, operator-managed restart helper (only for --restart-manager custom)")
 flag.Parse()
 server:=&webconfig.Server{Store:&configeditor.Store{Path:*path},PublicOrigin:*publicOrigin,TrustedProxyIP:*proxyIP,AllowLANHTTP:*allowLANHTTP}
 if *enableRestart {
  plan,err:=restartmanager.Resolve(*restartManager,*restartTarget,*restartHelper)
  if err!=nil{log.Fatalf("configure restart: %v",err)}
  log.Printf("Restart adapter enabled: %s (target configured at startup; OS permissions required)",plan.Manager)
  server.RestartService=plan.Run
 } else if *restartManager!="auto" || *restartTarget!="" || *restartHelper!="" {
  log.Fatal("restart options require --enable-restart")
 }
 if *cert!="" || *key!="" {
  log.Printf("Sparky configuration UI HTTPS listener %s (login required)",*addr)
  log.Fatal(server.ListenAndServeTLS(*addr,*cert,*key))
 }
 log.Printf("Sparky configuration UI HTTP listener %s (login required, LAN proxy opt-in: %t)",*addr,*allowLANHTTP)
 log.Fatal(server.ListenAndServe(*addr))
}
