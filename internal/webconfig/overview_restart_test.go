package webconfig

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

// The overview must never advertise a non-existent restart control or show
// a stale "coming later" banner after a restart adapter is enabled.
func TestOverviewRestartActionMatchesOperatorConfiguration(t *testing.T){
 for _,tc:=range []struct{name string;enabled bool}{
  {"disabled-by-default",false},
  {"enabled",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   s:=testServer(t)
   if tc.enabled{s.RestartService=func(context.Context)error{return nil}}
   request:=localTestRequest(http.MethodGet,"http://localhost:8080/",nil)
   request.SetBasicAuth("admin","this-is-a-long-test-password")
   response:=httptest.NewRecorder()
   s.ServeHTTP(response,request)
   if response.Code!=http.StatusOK{t.Fatalf("overview returned %d",response.Code)}
   html:=response.Body.String()
   if strings.Contains(html,"Service restart will be enabled in a later stage"){
    t.Fatal("overview advertises removed restart placeholder")
   }
   if strings.Contains(html,"Service restart is deliberately not available yet"){
    t.Fatal("overview contains stale restart guidance")
   }
   hasLink:=strings.Contains(html,`href="/edit#restart"`)
   if hasLink!=tc.enabled{t.Fatalf("restart navigation presence=%t, enabled=%t",hasLink,tc.enabled)}
   hasDisabled:=strings.Contains(html,"Restart disabled by operator")
   if hasDisabled==tc.enabled{t.Fatalf("disabled state=%t, enabled=%t",hasDisabled,tc.enabled)}
   if strings.Contains(html,`action="/restart"`){t.Fatal("overview must not make restart requests")}
  })
 }
}

func TestRestartOverviewLinksToConfirmedEditorForm(t *testing.T){
 s:=testServer(t)
 s.RestartService=func(context.Context)error{return nil}
 req:=localTestRequest(http.MethodGet,"http://localhost:8080/edit",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 rec:=httptest.NewRecorder()
 s.ServeHTTP(rec,req)
 if rec.Code!=http.StatusOK{t.Fatalf("editor returned %d",rec.Code)}
 if !strings.Contains(rec.Body.String(),`id="restart"`){t.Fatal("restart target missing on editor")}
 if !strings.Contains(rec.Body.String(),`action="/restart"`){t.Fatal("confirmed restart form missing")}
 if !strings.Contains(rec.Body.String(),`name="confirm"`){t.Fatal("restart confirmation missing")}
}
