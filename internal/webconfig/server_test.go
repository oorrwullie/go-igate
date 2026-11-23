package webconfig

import (
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/oorrwullie/sparky/internal/configeditor"
)

func testServer(t *testing.T)*Server{
 t.Helper()
 path:=filepath.Join(t.TempDir(),"config.yml")
 doc:="station-callsign: N0CALL-10\nopenwebrx-mqtt:\n  enabled: true\n  password: MQTT_SUPER_SECRET\n  topic: openwebrx/APRS\nigate:\n  aprsis:\n    passcode: APRS_SUPER_SECRET\n  beacon:\n    aprsfi-verification:\n      api-key: KEY_SUPER_SECRET\nmap-flights:\n  key: ADSB_SUPER_SECRET\n"
 if err:=os.WriteFile(path,[]byte(doc),0600);err!=nil{t.Fatal(err)}
 return &Server{Store:&configeditor.Store{Path:path}}
}
func TestReadOnlyPageRedactsSecrets(t *testing.T){
 s:=testServer(t);req:=httptest.NewRequest("GET","http://127.0.0.1:8080/",nil)
 rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
 if rec.Code!=200{t.Fatalf("status: %d %s",rec.Code,rec.Body.String())}
 body:=rec.Body.String()
 for _,secret:=range []string{"MQTT_SUPER_SECRET","APRS_SUPER_SECRET","KEY_SUPER_SECRET","ADSB_SUPER_SECRET"}{
  if strings.Contains(body,secret){t.Fatalf("secret leaked: %s",secret)}
 }
 if !strings.Contains(body,"N0CALL-10"){t.Fatal("callsign absent")}
 if !strings.Contains(rec.Header().Get("Cache-Control"),"no-store"){t.Fatal("not cache protected")}
}
func TestRejectExternalHostAndMutations(t *testing.T){
 s:=testServer(t)
 cases:=[]struct{host,method string;code int}{
  {"sparky.example.com:8080","GET",403},
  {"localhost.evil.com:8080","GET",403},
  {"127.0.0.1:8080","POST",405},
  {"localhost:8080","PUT",405},
 }
 for _,tc:=range cases{
  req:=httptest.NewRequest(tc.method,"http://127.0.0.1:8080/",nil);req.Host=tc.host
  rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
  if rec.Code!=tc.code{t.Errorf("%s %s: got %d, want %d",tc.host,tc.method,rec.Code,tc.code)}
 }
}
func TestInvalidConfigDoesNotExposeContents(t *testing.T){
 p:=filepath.Join(t.TempDir(),"config.yml")
 if err:=os.WriteFile(p,[]byte("transmitter:\n  audio-level: 7\npassword: VERY_SECRET"),0600);err!=nil{t.Fatal(err)}
 s:=&Server{Store:&configeditor.Store{Path:p}}
 rec:=httptest.NewRecorder();s.ServeHTTP(rec,httptest.NewRequest("GET","http://localhost:8080/",nil))
 if rec.Code!=http.StatusServiceUnavailable||strings.Contains(rec.Body.String(),"VERY_SECRET"){t.Fatal("invalid configuration disclosed")}
}
func TestLoopbackHostValidation(t *testing.T){
 for _,host:=range []string{"127.0.0.1:8080","[::1]:8080","localhost:8080"}{if !localHost(host){t.Errorf("rejected %q",host)}}
 for _,host:=range []string{"0.0.0.0:8080","192.168.1.22:8080","evil.com:8080"}{if localHost(host){t.Errorf("accepted %q",host)}}
}
