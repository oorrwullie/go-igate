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
 flag.Parse()
 server:=&webconfig.Server{Store:&configeditor.Store{Path:*path}}
 if *cert!="" || *key!="" {
  log.Printf("Sparky configuration UI HTTPS listener %s (login required)",*addr)
  log.Fatal(server.ListenAndServeTLS(*addr,*cert,*key))
 }
 log.Printf("Sparky configuration UI HTTP listener %s (loopback only; login required)",*addr)
 log.Fatal(server.ListenAndServe(*addr))
}
