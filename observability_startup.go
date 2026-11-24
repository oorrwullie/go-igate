package main

import (
 "errors"
 "fmt"
 "net"
 "strings"

 "github.com/oorrwullie/sparky/internal/observability"
)

// prepareGateway binds the optional metrics socket BEFORE opening receiver
// hardware or starting child processes. A bind failure never invokes factory.
func prepareGateway(addr string,allowRemote bool,factory func()(*Sparky,error))(*Sparky,*observability.Registry,net.Listener,error){
 addr=strings.TrimSpace(addr)
 var listener net.Listener
 var registry *observability.Registry
 if addr!="" {
  host,_,err:=net.SplitHostPort(addr)
  if err!=nil{return nil,nil,nil,fmt.Errorf("invalid SPARKY_OBSERVABILITY_ADDR: %w",err)}
  ip:=net.ParseIP(host)
  if ip==nil{return nil,nil,nil,errors.New("SPARKY_OBSERVABILITY_ADDR requires a literal IP")}
  if !ip.IsLoopback()&&!allowRemote{return nil,nil,nil,errors.New("remote metrics exposure requires SPARKY_OBSERVABILITY_ALLOW_REMOTE=1 and network access controls")}
  listener,err=net.Listen("tcp",addr)
  if err!=nil{return nil,nil,nil,fmt.Errorf("observability listener: %w",err)}
  registry=observability.New()
 }
 gateway,err:=factory()
 if err!=nil{
  if listener!=nil{_ = listener.Close()}
  return nil,nil,nil,err
 }
 if registry!=nil{gateway.SetObservability(registry)}
 return gateway,registry,listener,nil
}
