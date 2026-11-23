package nicerf

import (
 "context"
 "errors"
 "io"
 "net"
 "strings"
 "sync"
 "testing"
 "time"
)

func connected(t *testing.T)(*Transport,net.Conn){
 t.Helper()
 a,b:=net.Pipe()
 transport,err:=NewTransport(a)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_=transport.Close();_=b.Close()})
 return transport,b
}

func TestTransportPartialResponseAndSuccess(t *testing.T){
 transport,peer:=connected(t)
 server:=make(chan error,1)
 go func(){
  buf:=make([]byte,len(Handshake()))
  _,err:=io.ReadFull(peer,buf)
  if err==nil&&string(buf)!=Handshake(){err=errors.New("unexpected command")}
  if err==nil{
   for _,b:=range []byte("+DMOCONNECT:0\r\n"){
    _,err=peer.Write([]byte{b})
    if err!=nil{break}
   }
  }
  server<-err
 }()
 if err:=transport.Exchange(context.Background(),Handshake(),time.Second);err!=nil{t.Fatal(err)}
 if err:=<-server;err!=nil{t.Fatal(err)}
}

func TestTransportSequentialExchanges(t *testing.T){
 transport,peer:=connected(t)
 go func(){
  for i:=0;i<2;i++{
   buf:=make([]byte,len(Handshake()))
   if _,err:=io.ReadFull(peer,buf);err!=nil{return}
   _,_=peer.Write([]byte("+DMOCONNECT:0\r\n"))
  }
 }()
 var wg sync.WaitGroup
 for i:=0;i<2;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   if err:=transport.Exchange(context.Background(),Handshake(),time.Second);err!=nil{
    t.Errorf("sequential exchange failed: %v",err)
   }
  }()
 }
 wg.Wait()
}

func TestTransportTimeoutPoisonsSession(t *testing.T){
 transport,peer:=connected(t)
 go func(){_,_=io.CopyN(io.Discard,peer,int64(len(Handshake())))}()
 err:=transport.Exchange(context.Background(),Handshake(),100*time.Millisecond)
 if !errors.Is(err,context.DeadlineExceeded)&&!strings.Contains(err.Error(),"timeout"){t.Fatalf("unexpected timeout: %v",err)}
 if err:=transport.Exchange(context.Background(),Handshake(),time.Second);!errors.Is(err,ErrUnusable){
  t.Fatalf("late response could be misidentified: %v",err)
 }
}

func TestTransportCancellationBeforeWrite(t *testing.T){
 transport,_:=connected(t)
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 if err:=transport.Exchange(ctx,Handshake(),time.Second);!errors.Is(err,context.Canceled){t.Fatalf("got %v",err)}
}

func TestTransportCancellationDuringRead(t *testing.T){
 transport,peer:=connected(t)
 ctx,cancel:=context.WithCancel(context.Background())
 defer cancel()
 received:=make(chan struct{})
 go func(){
  buf:=make([]byte,len(Handshake()))
  _,_=io.ReadFull(peer,buf)
  close(received)
 }()
 result:=make(chan error,1)
 go func(){result<-transport.Exchange(ctx,Handshake(),time.Second)}()
 select{case <-received:case <-time.After(time.Second):t.Fatal("command not written")}
 cancel()
 select{
 case err:=<-result:
  if !errors.Is(err,context.Canceled){t.Fatalf("got %v",err)}
 case <-time.After(300*time.Millisecond):t.Fatal("cancellation did not interrupt read")
 }
}

func TestTransportRejectsWrongResponseAndNeverReusesSession(t *testing.T){
 transport,peer:=connected(t)
 go func(){
  buf:=make([]byte,len(Handshake()))
  _,_=io.ReadFull(peer,buf)
  _,_=peer.Write([]byte("+DMOSETGROUP:0\r\n"))
 }()
 if err:=transport.Exchange(context.Background(),Handshake(),time.Second);err==nil{t.Fatal("wrong acknowledgement accepted")}
 if err:=transport.Exchange(context.Background(),Handshake(),time.Second);!errors.Is(err,ErrUnusable){
  t.Fatalf("bad acknowledgement did not poison session: %v",err)
 }
}

func TestTransportCloseInterruptsBlockedRead(t *testing.T){
 transport,peer:=connected(t)
 received:=make(chan struct{})
 go func(){
  buf:=make([]byte,len(Handshake()))
  _,_=io.ReadFull(peer,buf)
  close(received)
 }()
 result:=make(chan error,1)
 go func(){result<-transport.Exchange(context.Background(),Handshake(),2*time.Second)}()
 select{case <-received:case <-time.After(time.Second):t.Fatal("no command")}
 if err:=transport.Close();err!=nil{t.Fatal(err)}
 select{
 case err:=<-result:if err==nil{t.Fatal("closed transport succeeded")}
 case <-time.After(300*time.Millisecond):t.Fatal("Close did not unblock read")
 }
 if err:=transport.Exchange(context.Background(),Handshake(),time.Second);err==nil{t.Fatal("exchange after close succeeded")}
}

func TestTransportRejectsInvalidCommandBeforeIO(t *testing.T){
 transport,_:=connected(t)
 for _,cmd:=range []string{"AT+UNKNOWN\r\n","AT+DMOCONNECT\n","AT+DMOCONNECT\r\nBAD\r\n"}{
  if err:=transport.Exchange(context.Background(),cmd,time.Second);err==nil{t.Fatalf("accepted malformed command %q",cmd)}
 }
 if err:=transport.Exchange(context.Background(),Handshake(),-1);err==nil{t.Fatal("accepted negative timeout")}
}

func TestTransportRejectsOversizedResponse(t *testing.T){
 transport,peer:=connected(t)
 go func(){
  buf:=make([]byte,len(Handshake()))
  _,_=io.ReadFull(peer,buf)
  _,_=peer.Write([]byte(strings.Repeat("X",maxResponseBytes)))
 }()
 if err:=transport.Exchange(context.Background(),Handshake(),time.Second);err==nil||!strings.Contains(err.Error(),"exceeds"){
  t.Fatalf("oversized response: %v",err)
 }
}
