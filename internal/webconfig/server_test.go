package webconfig

import (
 "net/http"
 "crypto/tls"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/oorrwullie/sparky/internal/configeditor"
)

func testServer(t *testing.T)*Server{
 t.Helper()
 t.Setenv("SPARKY_CONFIG_PASSWORD","this-is-a-long-test-password")
 path:=filepath.Join(t.TempDir(),"config.yml")
 doc:="station-callsign: N0CALL-10\nopenwebrx-mqtt:\n  enabled: true\n  password: MQTT_SUPER_SECRET\n  topic: openwebrx/APRS\nigate:\n  aprsis:\n    passcode: APRS_SUPER_SECRET\n  beacon:\n    aprsfi-verification:\n      api-key: KEY_SUPER_SECRET\nmap-flights:\n  key: ADSB_SUPER_SECRET\n"
 if err:=os.WriteFile(path,[]byte(doc),0600);err!=nil{t.Fatal(err)}
 return &Server{Store:&configeditor.Store{Path:path}}
}
func TestReadOnlyPageRedactsSecrets(t *testing.T){
 s:=testServer(t);req:=httptest.NewRequest("GET","http://127.0.0.1:8080/",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
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
  req.SetBasicAuth("admin","this-is-a-long-test-password")
  rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
  if rec.Code!=tc.code{t.Errorf("%s %s: got %d, want %d",tc.host,tc.method,rec.Code,tc.code)}
 }
}
func TestInvalidConfigDoesNotExposeContents(t *testing.T){
 p:=filepath.Join(t.TempDir(),"config.yml")
 if err:=os.WriteFile(p,[]byte("transmitter:\n  audio-level: 7\npassword: VERY_SECRET"),0600);err!=nil{t.Fatal(err)}
 t.Setenv("SPARKY_CONFIG_PASSWORD","this-is-a-long-test-password")
 s:=&Server{Store:&configeditor.Store{Path:p}}
 req:=httptest.NewRequest("GET","http://localhost:8080/",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
 if rec.Code!=http.StatusServiceUnavailable||strings.Contains(rec.Body.String(),"VERY_SECRET"){t.Fatal("invalid configuration disclosed")}
}
func TestLoopbackHostValidation(t *testing.T){
 for _,host:=range []string{"127.0.0.1:8080","[::1]:8080","localhost:8080"}{if !localHost(host){t.Errorf("rejected %q",host)}}
 for _,host:=range []string{"0.0.0.0:8080","192.168.1.22:8080","evil.com:8080"}{if localHost(host){t.Errorf("accepted %q",host)}}
}

// TestTemplateEscaping verifies that configuration strings cannot turn into
// browser-executable markup, even though the web UI is read-only.
func TestTemplateEscaping(t *testing.T) {
 p:=filepath.Join(t.TempDir(),"config.yml")
 marker:="<script>alert(1)</script>"
 if err:=os.WriteFile(p,[]byte("station-callsign: '"+marker+"'\n"),0600);err!=nil{t.Fatal(err)}
 t.Setenv("SPARKY_CONFIG_PASSWORD","this-is-a-long-test-password")
 s:=&Server{Store:&configeditor.Store{Path:p}}
 req:=httptest.NewRequest("GET","http://localhost:8080/",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 rec:=httptest.NewRecorder()
 s.ServeHTTP(rec,req)
 if rec.Code!=200{t.Fatalf("status %d",rec.Code)}
 if strings.Contains(rec.Body.String(),marker){t.Fatal("raw HTML was not escaped")}
 if !strings.Contains(rec.Body.String(),"&lt;script&gt;"){t.Fatal("HTML escaping missing")}
}

func TestBindAddressPolicy(t *testing.T) {
 s:=&Server{}
 for _,addr:=range []string{"0.0.0.0:0","192.168.1.1:8080","localhost:8080",":8080"} {
  if err:=s.ListenAndServe(addr);err==nil{t.Fatalf("accepted nonliteral loopback bind %q",addr)}
 }
}

func TestEveryRouteRequiresCredentials(t *testing.T) {
 s:=testServer(t)
 for _,path:=range []string{"/","/edit","/not-found"} {
  for _,tc:=range []struct{name,user,password string;want int}{
   {"missing","","",http.StatusUnauthorized},
   {"wrong username","other","this-is-a-long-test-password",http.StatusUnauthorized},
   {"wrong password","admin","incorrect-long-test-password",http.StatusUnauthorized},
   {"correct","admin","this-is-a-long-test-password",0},
  } {
   t.Run(path+"/"+tc.name,func(t *testing.T){
    req:=httptest.NewRequest("GET","http://localhost:8080"+path,nil)
    if tc.user!="" {req.SetBasicAuth(tc.user,tc.password)}
    rec:=httptest.NewRecorder()
    s.ServeHTTP(rec,req)
    if tc.want!=0 && rec.Code!=tc.want {t.Fatalf("got %d want %d",rec.Code,tc.want)}
    if tc.want==0 && rec.Code==http.StatusUnauthorized {t.Fatalf("rejected valid auth")}
    if tc.want!=0 && strings.Contains(rec.Body.String(),"N0CALL-10") {t.Fatal("private content exposed")}
    if tc.want==http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate")=="" {t.Fatal("missing login challenge")}
   })
  }
 }
}

func TestPasswordMissingOrShortFailsClosed(t *testing.T) {
 s:=testServer(t)
 for _,pass:=range []string{"","weak"}{
  t.Setenv("SPARKY_CONFIG_PASSWORD",pass)
  req:=httptest.NewRequest("GET","http://localhost:8080/",nil)
  req.SetBasicAuth("admin",pass)
  rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
  if rec.Code!=http.StatusServiceUnavailable {t.Fatalf("password length %d: status %d",len(pass),rec.Code)}
 }
}

func TestRemoteHostRequiresHTTPS(t *testing.T) {
 s:=testServer(t)
 req:=httptest.NewRequest("GET","http://192.168.1.42:8443/",nil)
 req.Host="192.168.1.42:8443"
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
 if rec.Code!=http.StatusForbidden {t.Fatalf("remote HTTP accepted: %d",rec.Code)}

 req=httptest.NewRequest("GET","https://192.168.1.42:8443/",nil)
 req.Host="192.168.1.42:8443"
 req.TLS=&tls.ConnectionState{Version:tls.VersionTLS13}
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 rec=httptest.NewRecorder();s.ServeHTTP(rec,req)
 if rec.Code!=http.StatusOK {t.Fatalf("remote HTTPS rejected: %d",rec.Code)}
}

func TestRemoteListenerRequiresTLSCredentials(t *testing.T) {
 s:=&Server{}
 for _,addr:=range []string{"0.0.0.0:0","192.168.1.42:8443","[::]:8443"} {
  if err:=s.ListenAndServe(addr);err==nil {t.Fatalf("accepted remote HTTP listener %q",addr)}
  if err:=s.ListenAndServeTLS(addr,"","");err==nil {t.Fatalf("accepted missing TLS files for %q",addr)}
 }
 if err:=s.ListenAndServeTLS("0.0.0.0:0","missing-cert.pem","missing-key.pem");err==nil {
  t.Fatal("accepted invalid TLS credential paths")
 }
}

func TestTrustedRoutyLANProxy(t *testing.T) {
 s:=testServer(t)
 s.PublicOrigin="https://sparky.example.org"
 s.TrustedProxyIP="192.168.1.10"
 s.AllowLANHTTP=true
 for _,tc:=range []struct{
  name,host,remote string;want int
 }{
  {"trusted Routy","sparky.example.org","192.168.1.10:45678",200},
  {"wrong source","sparky.example.org","192.168.1.20:45678",403},
  {"spoofed local hostname","localhost:8080","192.168.1.20:45678",403},
  {"wrong hostname","evil.example.org","192.168.1.10:45678",403},
 } {
  t.Run(tc.name,func(t *testing.T){
   req:=httptest.NewRequest("GET","http://127.0.0.1:8080/",nil)
   req.Host=tc.host
   req.RemoteAddr=tc.remote
   req.SetBasicAuth("admin","this-is-a-long-test-password")
   rec:=httptest.NewRecorder();s.ServeHTTP(rec,req)
   if rec.Code!=tc.want {t.Fatalf("%s returned %d want %d",tc.name,rec.Code,tc.want)}
  })
 }
}

func TestPublicOriginValidation(t *testing.T) {
 for _,origin:=range []string{"http://sparky.example.org","https://sparky.example.org/path","https://user@sparky.example.org","https://sparky.example.org?x=y","https://"} {
  if _,err:=parsePublicOrigin(origin);err==nil {t.Fatalf("accepted invalid origin %q",origin)}
 }
 if _,err:=parsePublicOrigin("https://sparky.example.org");err!=nil {t.Fatal(err)}
}

func TestLANOptInRejectsIncompleteConfiguration(t *testing.T) {
 for _,tc:=range []struct{
  name string;server Server
 }{
  {"no origin",Server{AllowLANHTTP:true,TrustedProxyIP:"192.168.1.10"}},
  {"no proxy IP",Server{AllowLANHTTP:true,PublicOrigin:"https://sparky.example.org"}},
  {"invalid proxy IP",Server{AllowLANHTTP:true,PublicOrigin:"https://sparky.example.org",TrustedProxyIP:"Routy"}},
 } {
  t.Run(tc.name,func(t *testing.T){
   if err:=tc.server.ListenAndServe("0.0.0.0:0");err==nil {t.Fatal("accepted unsafe LAN HTTP configuration")}
  })
 }
}
