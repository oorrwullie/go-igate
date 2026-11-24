package webconfig

import (
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestValidationFeedbackNoSecretDisclosure(t *testing.T) {
 cases:=[]struct{cause error;want string}{
  {errors.New("invalid configuration: map-flights requires openwebrx-mqtt.enabled"),"requires OpenWebRX MQTT"},
  {errors.New("invalid configuration: map-flights.station and map-flights.key are required"),"requires station ID"},
  {errors.New("invalid configuration: transmitter audio-level must be between 0 and 1"),"between 0 and 1"},
  {errors.New("secret value leaked: PRIVATE_API_KEY_abc"),"Inspect YAML"},
  {errors.New("open /tmp/map-flights.broker/config.yml: permission denied"),"Inspect YAML"},
  {errors.New("mkdir /tmp/map-flights requires/failed: permission denied"),"Inspect YAML"},
  {errors.New("failed to stat transmitter audio-level /tmp/private"),"Inspect YAML"},
 }
 for _,tc:=range cases {
  got:=validationFeedback(tc.cause)
  if !strings.Contains(got,tc.want){t.Errorf("feedback %q does not contain %q",got,tc.want)}
  if strings.Contains(got,"PRIVATE_API_KEY_abc"){t.Fatal("secret was echoed in validation feedback")}
 }
}

func TestEditorScriptRequiresLoginAndCorrectMethods(t *testing.T) {
 s:=testServer(t)
 url:="http://localhost:8080/assets/editor.js"
 unauth:=localTestRequest(http.MethodGet,url,nil)
 w:=httptest.NewRecorder()
 s.ServeHTTP(w,unauth)
 if w.Code!=http.StatusUnauthorized{t.Fatalf("script accessible without login: %d",w.Code)}

 req:=localTestRequest(http.MethodGet,url,nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 w=httptest.NewRecorder()
 s.ServeHTTP(w,req)
 if w.Code!=http.StatusOK{t.Fatalf("script status: %d",w.Code)}
 if !strings.Contains(w.Body.String(),"beforeunload"){t.Fatal("missing dirty form guard")}
 if !strings.Contains(w.Body.String(),`addEventListener("pageshow"`) {t.Fatal("missing bfcache restoration guard")}
 if !strings.HasPrefix(w.Header().Get("Content-Type"),"text/javascript"){t.Fatal("unexpected script content type")}
 if w.Header().Get("Cache-Control")!="no-store"{t.Fatal("script caching not disabled")}

 post:=localTestRequest(http.MethodPost,url,nil)
 post.SetBasicAuth("admin","this-is-a-long-test-password")
 w=httptest.NewRecorder()
 s.ServeHTTP(w,post)
 if w.Code!=http.StatusMethodNotAllowed{t.Fatalf("script permits POST: %d",w.Code)}
}

func TestEditorPageShowsRestartStatusAfterSave(t *testing.T) {
 s:=testServer(t)
 req:=localTestRequest(http.MethodGet,"http://localhost:8080/edit?saved=1",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 w:=httptest.NewRecorder()
 s.ServeHTTP(w,req)
 if w.Code!=200{t.Fatalf("editor status: %d",w.Code)}
 body:=w.Body.String()
 for _,want:=range []string{"RESTART REQUIRED","data-dirty-check","src=\"/assets/editor.js\""}{
  if !strings.Contains(body,want){t.Errorf("missing %q",want)}
 }
 if !strings.Contains(w.Header().Get("Content-Security-Policy"),"script-src 'self'"){t.Fatal("editor CSP blocks script")}
}
