package webconfig

import (
 "context"
 "encoding/hex"
 "errors"
 "net/http"
 "net"
 "time"
 "io"
 "net/http/httptest"
 "net/url"
 "strings"
 "sync/atomic"
 "testing"
)

func restartRequest(method,origin,token,confirmation string)*http.Request{
 form:=url.Values{"csrf":{token},"confirm":{confirmation}}
 req:=localTestRequest(method,"http://localhost:8080/restart",strings.NewReader(form.Encode()))
 req.Header.Set("Content-Type","application/x-www-form-urlencoded")
 req.Header.Set("Origin",origin)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 return req
}
func TestRestartDisabledByDefault(t *testing.T){
 s:=testServer(t)
 req:=restartRequest("POST","http://localhost:8080","","restart")
 w:=httptest.NewRecorder();s.ServeHTTP(w,req)
 if w.Code!=http.StatusNotFound{t.Fatalf("disabled restart returned %d",w.Code)}
 req=localTestRequest("GET","http://localhost:8080/edit",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 w=httptest.NewRecorder();s.ServeHTTP(w,req)
 if strings.Contains(w.Body.String(),`action="/restart"`){t.Fatal("disabled restart form shown")}
}
func TestRestartRequiresAuthOriginCSRFAndConfirmation(t *testing.T){
 s:=testServer(t)
 var calls atomic.Int32
 s.RestartService=func(context.Context)error{calls.Add(1);return nil}
 token,err:=csrfToken(s);if err!=nil{t.Fatal(err)}
 good:=hex.EncodeToString(token)
 cases:=[]struct{name,method,origin,csrf,confirm string;want int}{
  {"get","GET","http://localhost:8080",good,"restart",405},
  {"wrong origin","POST","http://other.example",good,"restart",403},
  {"null origin","POST","null",good,"restart",403},
  {"missing token","POST","http://localhost:8080","","restart",403},
  {"invalid token","POST","http://localhost:8080","bad","restart",403},
  {"no confirmation","POST","http://localhost:8080",good,"",400},
  {"valid","POST","http://localhost:8080",good,"restart",200},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   req:=restartRequest(tc.method,tc.origin,tc.csrf,tc.confirm)
   w:=httptest.NewRecorder();s.ServeHTTP(w,req)
   if w.Code!=tc.want{t.Fatalf("status %d want %d: %s",w.Code,tc.want,w.Body.String())}
  })
 }
 if calls.Load()!=1{t.Fatalf("restart called %d times",calls.Load())}
 req:=restartRequest("POST","http://localhost:8080",good,"restart")
 req.SetBasicAuth("admin","wrong-password")
 w:=httptest.NewRecorder();s.ServeHTTP(w,req)
 if w.Code!=401 || calls.Load()!=1{t.Fatal("unauthorized restart accepted")}
}
func TestRestartDoesNotLeakAdapterErrors(t *testing.T){
 s:=testServer(t)
 s.RestartService=func(context.Context)error{return errors.New("/secret/systemd-path/credentials")}
 token,_:=csrfToken(s)
 w:=httptest.NewRecorder()
 s.ServeHTTP(w,restartRequest("POST","http://localhost:8080",hex.EncodeToString(token),"restart"))
 if w.Code!=502{t.Fatalf("status %d",w.Code)}
 if strings.Contains(w.Body.String(),"/secret"){t.Fatal("adapter error leaked")}
}
func TestRestartPreventsConcurrentCommands(t *testing.T){
 s:=testServer(t)
 entered:=make(chan struct{})
 unblock:=make(chan struct{})
 var calls atomic.Int32
 s.RestartService=func(context.Context)error{
  calls.Add(1)
  close(entered)
  <-unblock
  return nil
 }
 token,_:=csrfToken(s)
 good:=hex.EncodeToString(token)
 finished:=make(chan struct{})
 go func(){
  defer close(finished)
  w:=httptest.NewRecorder()
  s.ServeHTTP(w,restartRequest("POST","http://localhost:8080",good,"restart"))
 }()
 <-entered
 w:=httptest.NewRecorder()
 s.ServeHTTP(w,restartRequest("POST","http://localhost:8080",good,"restart"))
 if w.Code!=409{t.Fatalf("concurrent restart returned %d",w.Code)}
 close(unblock)
 <-finished
 if calls.Load()!=1{t.Fatalf("concurrent restart executed %d times",calls.Load())}
}
func TestRestartFormVisibleOnlyWhenEnabled(t *testing.T){
 s:=testServer(t)
 s.RestartService=func(context.Context)error{return nil}
 req:=localTestRequest("GET","http://localhost:8080/edit",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 w:=httptest.NewRecorder();s.ServeHTTP(w,req)
 if w.Code!=200 || !strings.Contains(w.Body.String(),`action="/restart"`){
  t.Fatalf("enabled restart form absent: %d",w.Code)
 }
 if !strings.Contains(w.Body.String(),`name="confirm"`){t.Fatal("confirmation checkbox absent")}
}

func TestRestartLiveListenerCompletesBeyondOldWriteTimeout(t *testing.T) {
 s:=testServer(t)
 s.RestartService=func(context.Context)error {
  time.Sleep(11*time.Second)
  return nil
 }
 if got:=newHTTPServer(s).WriteTimeout;got<20*time.Second{
  t.Fatalf("enabled restart write timeout is %v",got)
 }
 if got:=newHTTPServer(&Server{}).WriteTimeout;got!=10*time.Second{
  t.Fatalf("default write timeout unexpectedly changed to %v",got)
 }
 listener,err:=net.Listen("tcp","127.0.0.1:0")
 if err!=nil{t.Fatal(err)}
 server:=newHTTPServer(s)
 done:=make(chan error,1)
 go func(){done<-server.Serve(listener)}()
 defer func(){
  _=server.Close()
  <-done
 }()
 token,err:=csrfToken(s)
 if err!=nil{t.Fatal(err)}
 address:="http://"+listener.Addr().String()
 form:=url.Values{"csrf":{hex.EncodeToString(token)},"confirm":{"restart"}}
 req,err:=http.NewRequest("POST",address+"/restart",strings.NewReader(form.Encode()))
 if err!=nil{t.Fatal(err)}
 req.Header.Set("Origin",address)
 req.Header.Set("Content-Type","application/x-www-form-urlencoded")
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 client:=&http.Client{Timeout:19*time.Second}
 resp,err:=client.Do(req)
 if err!=nil{t.Fatalf("live restart response failed: %v",err)}
 defer resp.Body.Close()
 body,err:=io.ReadAll(resp.Body)
 if err!=nil{t.Fatal(err)}
 if resp.StatusCode!=200{t.Fatalf("restart returned %d: %s",resp.StatusCode,body)}
 if !strings.Contains(string(body),"Restart requested"){t.Fatal("restart success response missing")}
}
