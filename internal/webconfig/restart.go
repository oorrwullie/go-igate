package webconfig

import (
 "context"
 "crypto/subtle"
 "encoding/hex"
 "net/http"
 "time"
)

// serveRestart is an opt-in, independently authorized operation. The browser
// cannot select the target unit or a command; the host wires a fixed adapter.
func (s *Server) serveRestart(w http.ResponseWriter, r *http.Request) {
 if s==nil || s.RestartService==nil {
  http.NotFound(w,r)
  return
 }
 if r.Method!=http.MethodPost {
  w.Header().Set("Allow", "POST")
  http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
  return
 }
 if r.Header.Get("Origin")!=s.expectedOrigin(r) {
  http.Error(w,"Invalid request origin",http.StatusForbidden)
  return
 }
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 if err:=r.ParseForm();err!=nil {
  http.Error(w,"Invalid request",http.StatusBadRequest)
  return
 }
 token,err:=csrfToken(s)
 if err!=nil {
  http.Error(w,"Restart unavailable",http.StatusServiceUnavailable)
  return
 }
 submitted,err:=hex.DecodeString(r.PostForm.Get("csrf"))
 if err!=nil || subtle.ConstantTimeCompare(submitted,token)!=1 {
  http.Error(w,"Invalid CSRF token",http.StatusForbidden)
  return
 }
 if r.PostForm.Get("confirm")!="restart" {
  http.Error(w,"Restart confirmation required",http.StatusBadRequest)
  return
 }
 if !s.restartInFlight.CompareAndSwap(false,true) {
  http.Error(w,"Restart already in progress",http.StatusConflict)
  return
 }
 defer s.restartInFlight.Store(false)
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second)
 defer cancel()
 if err:=s.RestartService(ctx);err!=nil {
  // Avoid disclosing filesystem paths, policy details or subprocess output.
  http.Error(w,"Restart was not confirmed. Check the service manager logs.",http.StatusBadGateway)
  return
 }
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 _,_=w.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>Sparky // Restart</title></head><body><h1>Restart requested</h1><p>The service manager accepted the restart operation. This does not verify that Sparky came back healthy.</p><a href=\"/\">Return to overview</a></body></html>"))
}
