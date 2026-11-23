// sparky-config is a loopback-only, read-only preview of Sparky configuration.
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
 log.Printf("Sparky read-only config preview on %s",*addr)
 log.Fatal((&webconfig.Server{Store:&configeditor.Store{Path:*path}}).ListenAndServe(*addr))
}
