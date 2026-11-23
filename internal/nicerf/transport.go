package nicerf

import (
 "context"
 "errors"
 "fmt"
 "io"
 "net"
 "strings"
 "sync"
 "time"
)

var (
 ErrClosed = errors.New("nicerf: serial transport closed")
 ErrUnusable = errors.New("nicerf: serial transport unusable after failed exchange")
)

// DeadlinePort is an already-open byte stream supporting read/write deadlines.
// Actual serial adapters must provide these guarantees; plain os.File doesn't.
// This package does not open UART, GPIO, PTT, or RF hardware.
type DeadlinePort interface {
 io.ReadWriteCloser
 SetReadDeadline(time.Time) error
 SetWriteDeadline(time.Time) error
}

// Transport owns the port and serializes commands. Any failed or partial
// exchange closes the session so a late response cannot acknowledge a later command.
type Transport struct {
 port DeadlinePort
 token chan struct{}
 closed chan struct{}
 once sync.Once
 mu sync.Mutex
 unusable bool
}

const (
 DefaultExchangeTimeout = 2 * time.Second
 maxResponseBytes = 96
 pollInterval = 50 * time.Millisecond
)

func NewTransport(port DeadlinePort) (*Transport,error) {
 if port == nil { return nil,errors.New("nicerf: nil serial port") }
 t := &Transport{port:port,token:make(chan struct{},1),closed:make(chan struct{})}
 t.token <- struct{}{}
 return t,nil
}

// Close interrupts in-flight reads and writes; further exchanges are rejected.
func (t *Transport) Close() error {
 var err error
 t.once.Do(func(){ close(t.closed);err=t.port.Close() })
 return err
}

func (t *Transport) fail() {
 t.mu.Lock()
 t.unusable=true
 t.mu.Unlock()
 _=t.Close()
}

func (t *Transport) check() error {
 t.mu.Lock()
 defer t.mu.Unlock()
 if t.unusable { return ErrUnusable }
 select {
 case <-t.closed: return ErrClosed
 default:
 }
 return nil
}

// Exchange writes one codec command and checks its CRLF response.
// Timeout zero selects DefaultExchangeTimeout; all I/O shares one absolute
// deadline and polls cancellation at most every 50ms. Errors are terminal.
func (t *Transport) Exchange(ctx context.Context,command string,timeout time.Duration) error {
 if ctx==nil { return errors.New("nicerf: nil context") }
 if timeout<0 { return errors.New("nicerf: negative timeout") }
 if timeout==0 { timeout=DefaultExchangeTimeout }
 if !strings.HasSuffix(command,"\r\n")||strings.Contains(command[:len(command)-2],"\r")||strings.Contains(command[:len(command)-2],"\n") {
  return errors.New("nicerf: malformed command framing")
 }
 switch {
 case command==Handshake(),command==FiltersOff():
 case strings.HasPrefix(command,"AT+DMOSETGROUP="):
 default: return errors.New("nicerf: unsupported command")
 }
 if err:=t.check();err!=nil{return err}
 select {
 case <-ctx.Done():return ctx.Err()
 case <-t.closed:return ErrClosed
 case <-t.token:
 }
 defer func(){t.token<-struct{}{}}()
 if err:=t.check();err!=nil{return err}
 deadline:=time.Now().Add(timeout)
 if d,ok:=ctx.Deadline();ok&&d.Before(deadline){deadline=d}
 write:=[]byte(command)
 for len(write)>0 {
  if err:=t.step(ctx,deadline,true);err!=nil{t.fail();return err}
  n,err:=t.port.Write(write)
  if n<0||n>len(write){t.fail();return errors.New("nicerf: invalid write count")}
  write=write[n:]
  if err!=nil{
   if isTimeout(err)&&time.Now().Before(deadline)&&ctx.Err()==nil{continue}
   t.fail();return fmt.Errorf("nicerf: write: %w",err)
  }
  if n==0 {t.fail();return io.ErrNoProgress}
 }
 var reply []byte
 var one [1]byte
 for len(reply)<maxResponseBytes{
  if err:=t.step(ctx,deadline,false);err!=nil{t.fail();return err}
  n,err:=t.port.Read(one[:])
  if n<0||n>1{t.fail();return errors.New("nicerf: invalid read count")}
  if n==1{
   reply=append(reply,one[0])
   if one[0]=='\n'{
    if len(reply)<2||reply[len(reply)-2]!='\r'{t.fail();return errors.New("nicerf: response missing CRLF")}
    if err:=ValidateResponse(command,string(reply));err!=nil{t.fail();return fmt.Errorf("nicerf: response: %w",err)}
    return nil
   }
  }
  if err!=nil{
   if isTimeout(err)&&time.Now().Before(deadline)&&ctx.Err()==nil{continue}
   t.fail();return fmt.Errorf("nicerf: read: %w",err)
  }
  if n==0 {t.fail();return io.ErrNoProgress}
 }
 t.fail()
 return fmt.Errorf("nicerf: response exceeds %d bytes",maxResponseBytes)
}

func (t *Transport) step(ctx context.Context,deadline time.Time,write bool) error{
 if err:=t.check();err!=nil{return err}
 if err:=ctx.Err();err!=nil{return err}
 now:=time.Now()
 if !now.Before(deadline){return context.DeadlineExceeded}
 d:=now.Add(pollInterval)
 if deadline.Before(d){d=deadline}
 var err error
 if write {err=t.port.SetWriteDeadline(d)} else {err=t.port.SetReadDeadline(d)}
 if err!=nil{return fmt.Errorf("nicerf: set I/O deadline: %w",err)}
 return nil
}

func isTimeout(err error) bool {
 var nerr net.Error
 return errors.As(err,&nerr)&&nerr.Timeout()
}
