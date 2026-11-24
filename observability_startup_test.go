package main

import (
 "errors"
 "net"
 "testing"
)

// An occupied metrics socket must fail before the receiver factory can
// acquire SDR hardware or start rtl_fm/multimon-ng subprocesses.
func TestObservabilityBindFailurePreventsReceiverAcquisition(t *testing.T){
 occupied,err:=net.Listen("tcp","127.0.0.1:0")
 if err!=nil{t.Fatal(err)}
 defer occupied.Close()
 acquired:=false
 factory:=func()(*Sparky,error){acquired=true;return &Sparky{},nil}
 gateway,registry,listener,err:=prepareGateway(occupied.Addr().String(),false,factory)
 if err==nil||acquired||gateway!=nil||registry!=nil||listener!=nil{
  t.Fatalf("bind failure did not precede acquisition: acquired=%t err=%v",acquired,err)
 }
}

func TestObservabilityPreflightFailurePreventsReceiverAcquisition(t *testing.T){
 for _,address:=range []string{"host.example:9091","0.0.0.0:9091","invalid-address"}{
  acquired:=false
  _,_,_,err:=prepareGateway(address,false,func()(*Sparky,error){
   acquired=true;return &Sparky{},nil
  })
  if err==nil||acquired{t.Errorf("%q allowed factory invocation: %v",address,err)}
 }
}

func TestObservabilityClosesListenerOnReceiverFailure(t *testing.T){
 probe,err:=net.Listen("tcp","127.0.0.1:0")
 if err!=nil{t.Fatal(err)}
 address:=probe.Addr().String()
 _=probe.Close()
 _,_,listener,err:=prepareGateway(address,false,func()(*Sparky,error){return nil,errors.New("radio unavailable")})
 if err==nil||listener!=nil{t.Fatal("failed init left open listener")}
 rebound,err:=net.Listen("tcp",address)
 if err!=nil{t.Fatalf("listener leaked after receiver failure: %v",err)}
 _=rebound.Close()
}

func TestObservabilityDisabledHasNoListener(t *testing.T){
 acquired:=false
 g,r,l,err:=prepareGateway("",false,func()(*Sparky,error){
  acquired=true;return &Sparky{},nil
 })
 if err!=nil||!acquired||g==nil||r!=nil||l!=nil{
  t.Fatalf("disabled observability changed gateway initialization: %v",err)
 }
}
