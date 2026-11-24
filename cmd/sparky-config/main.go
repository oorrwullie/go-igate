// sparky-config serves the authenticated Sparky configuration UI.
package main

import (
 "flag"
 "log"

 "github.com/oorrwullie/sparky/internal/configeditor"
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
 metricsURL:=flag.String("metrics-url","","optional local gateway O11y endpoint, e.g. http://127.0.0.1:9091/metrics")
 flag.Parse()
 if err:=webconfig.ValidateMetricsURL(*metricsURL);err!=nil{log.Fatalf("invalid metrics URL: %v",err)}
 server:=&webconfig.Server{Store:&configeditor.Store{Path:*path},PublicOrigin:*publicOrigin,TrustedProxyIP:*proxyIP,AllowLANHTTP:*allowLANHTTP,MetricsURL:*metricsURL}
 if *cert!="" || *key!="" {
  log.Printf("Sparky configuration UI HTTPS listener %s (login required)",*addr)
  log.Fatal(server.ListenAndServeTLS(*addr,*cert,*key))
 }
 log.Printf("Sparky configuration UI HTTP listener %s (login required, LAN proxy opt-in: %t)",*addr,*allowLANHTTP)
 log.Fatal(server.ListenAndServe(*addr))
}
