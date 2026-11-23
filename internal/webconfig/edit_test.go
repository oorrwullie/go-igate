package webconfig

import (
 "encoding/hex"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "github.com/oorrwullie/sparky/internal/configeditor"
)

func TestEditRequiresAuthentication(t *testing.T){
 t.Setenv("SPARKY_CONFIG_PASSWORD","this-is-a-long-test-password")
 s:=testServer(t);rec:=httptest.NewRecorder()
 s.ServeHTTP(rec,httptest.NewRequest("GET","http://localhost:8080/edit",nil))
 if rec.Code!=401{t.Fatalf("unexpected status %d",rec.Code)}
}
func TestEditPreservesSecretsAndRejectsCSRF(t *testing.T){
 t.Setenv("SPARKY_CONFIG_PASSWORD","this-is-a-long-test-password")
 p:=filepath.Join(t.TempDir(),"config.yml")
 initial:="# note retained\nstation-callsign: N0CALL\nopenwebrx-mqtt:\n  enabled: false\n  password: test-super-secret\n"
 if err:=os.WriteFile(p,[]byte(initial),0600);err!=nil{t.Fatal(err)}
 s:=&Server{Store:&configeditor.Store{Path:p}}
 req:=httptest.NewRequest("GET","http://localhost:8080/edit",nil);req.SetBasicAuth("admin","this-is-a-long-test-password")
 rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
 if rec.Code!=200{t.Fatalf("GET returned %d",rec.Code)}
 if strings.Contains(rec.Body.String(),"test-super-secret"){t.Fatal("secret leaked")}
 token,err:=csrfToken(s);if err!=nil{t.Fatal(err)}
 _,rev,err:=s.Store.Load();if err!=nil{t.Fatal(err)}
 form:=url.Values{"csrf":{hex.EncodeToString(token)},"revision":{rev},"callsign":{"N0CALL-10"},"igate":{"on"}}
 post:=func(origin string,form url.Values) *httptest.ResponseRecorder {
  r:=httptest.NewRequest("POST","http://localhost:8080/edit",strings.NewReader(form.Encode()))
  r.Header.Set("Content-Type","application/x-www-form-urlencoded")
  r.Header.Set("Origin",origin)
  r.SetBasicAuth("admin","this-is-a-long-test-password")
  w:=httptest.NewRecorder();s.ServeHTTP(w,r);return w
 }
 if w:=post("http://bad.example",form);w.Code!=403{t.Fatalf("cross-site post accepted: %d",w.Code)}
 if w:=post("http://localhost:8080",form);w.Code!=303{t.Fatalf("valid save returned %d: %s",w.Code,w.Body.String())}
 saved,_,err:=s.Store.Load();if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(saved),"test-super-secret")||!strings.Contains(string(saved),"# note retained")||!strings.Contains(string(saved),"N0CALL-10"){t.Fatalf("lost settings or comments: %s",saved)}
 if w:=post("http://localhost:8080",form);w.Code!=409{t.Fatalf("stale revision returned %d",w.Code)}
}
func TestEditableRejectsInvalidNestedType(t *testing.T){
 if _,err:=patchEditable([]byte("igate: this-should-be-a-map\n"),editValues{});err==nil{t.Fatal("overwrote non-map igate field")}
}
