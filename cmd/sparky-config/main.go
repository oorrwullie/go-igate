// sparky-config is a password-protected, loopback-only Sparky configuration UI.
package main

import (
 "flag"
 "log"

 "github.com/oorrwullie/sparky/internal/configeditor"
 "github.com/oorrwullie/sparky/internal/webconfig"
)

func main() {
 addr:=flag.String("listen","127.0.0.1:8080","loopback TCP address")
 path:=flag.String("config","config.yml","path to Sparky configuration")
 flag.Parse()
 log.Printf("Sparky configuration UI listening on %s (loopback; password required)",*addr)
 log.Fatal((&webconfig.Server{Store:&configeditor.Store{Path:*path}}).ListenAndServe(*addr))
}
