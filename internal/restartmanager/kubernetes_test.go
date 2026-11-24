package restartmanager

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestKubernetesRolloutRestartUsesFixedDeploymentAndNamespace(t *testing.T){
 var calls int
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls++
  if r.Method!=http.MethodPatch{t.Errorf("method %s",r.Method)}
  if got:=r.URL.Path;got!="/apis/apps/v1/namespaces/radio/deployments/sparky" {t.Errorf("path %s",got)}
  if r.Header.Get("Authorization")!="Bearer test-serviceaccount-token"{t.Error("wrong token")}
  if r.Header.Get("Content-Type")!="application/merge-patch+json"{t.Error("wrong patch media type")}
  var patch map[string]any
  if err:=json.NewDecoder(r.Body).Decode(&patch);err!=nil{t.Error(err)}
  spec,_:=patch["spec"].(map[string]any)
  template,_:=spec["template"].(map[string]any)
  metadata,_:=template["metadata"].(map[string]any)
  annotations,_:=metadata["annotations"].(map[string]any)
  if _,ok:=annotations["sparky.io/restarted-at"].(string);!ok{t.Error("missing rollout timestamp")}
  w.WriteHeader(http.StatusOK)
 }))
 defer server.Close()
 tokenFile:=filepath.Join(t.TempDir(),"token")
 if err:=os.WriteFile(tokenFile,[]byte("test-serviceaccount-token\n"),0600);err!=nil{t.Fatal(err)}
 config:=KubernetesConfig{ServerURL:server.URL,Namespace:"radio",Deployment:"sparky",TokenPath:tokenFile,Client:server.Client()}
 if err:=config.Restart(context.Background());err!=nil{t.Fatal(err)}
 if calls!=1{t.Fatalf("calls: %d",calls)}
}

func TestKubernetesRestartRejectsRBACAndDoesNotEchoBody(t *testing.T){
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  http.Error(w,"secret serviceaccount detail",http.StatusForbidden)
 }))
 defer server.Close()
 tokenFile:=filepath.Join(t.TempDir(),"token")
 if err:=os.WriteFile(tokenFile,[]byte("test"),0600);err!=nil{t.Fatal(err)}
 config:=KubernetesConfig{ServerURL:server.URL,Namespace:"radio",Deployment:"sparky",TokenPath:tokenFile,Client:server.Client()}
 err:=config.Restart(context.Background())
 if err==nil||!strings.Contains(err.Error(),"403") {t.Fatalf("expected forbidden, got %v",err)}
 if strings.Contains(err.Error(),"secret"){t.Fatal("API error body leaked")}
}

func TestKubernetesServiceNamesValidated(t *testing.T){
 for _,name:=range []string{"sparky","my-sparky-2","s"}{
  if !validKubernetesName(name){t.Errorf("valid name rejected: %s",name)}
 }
 for _,name:=range []string{"","-bad","bad-","SomeService","bad/name","..","bad.name","a;rm -rf"}{
  if validKubernetesName(name){t.Errorf("invalid name accepted: %q",name)}
 }
}

func TestKubernetesEndpointCannotUseHTTP(t *testing.T){
 p:=filepath.Join(t.TempDir(),"token")
 if err:=os.WriteFile(p,[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 client:=&http.Client{}
 config:=KubernetesConfig{ServerURL:"http://localhost:8080",Namespace:"radio",Deployment:"sparky",TokenPath:p,Client:client}
 if err:=config.Restart(context.Background());err==nil{t.Fatal("HTTP endpoint accepted")}
}
