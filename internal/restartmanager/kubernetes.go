package restartmanager

import (
 "bytes"
 "context"
 "crypto/tls"
 "crypto/x509"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "os"
 "path"
 "strconv"
 "strings"
 "time"
)

// KubernetesConfig is fixed at UI startup. The workload is restarted via
// Deployment rollout rather than by killing a possibly unrelated PID.
type KubernetesConfig struct {
 ServerURL string
 Namespace string
 Deployment string
 TokenPath string
 CAPath string
 Client *http.Client
}

const serviceAccountDir="/var/run/secrets/kubernetes.io/serviceaccount"

func kubernetesPlan(target string)(Plan,error){
 if target=="" {target="deployment/sparky"}
 if !strings.HasPrefix(target,"deployment/") {return Plan{},errors.New("Kubernetes restart target must be deployment/<name>")}
 name:=strings.TrimPrefix(target,"deployment/")
 if !validKubernetesName(name){return Plan{},errors.New("invalid Kubernetes deployment name")}
 nsRaw,err:=os.ReadFile(serviceAccountDir+"/namespace")
 if err!=nil{return Plan{},fmt.Errorf("Kubernetes service account namespace unavailable: %w",err)}
 namespace:=strings.TrimSpace(string(nsRaw))
 if !validKubernetesName(namespace){return Plan{},errors.New("invalid Kubernetes namespace")}
 host:=os.Getenv("KUBERNETES_SERVICE_HOST")
 port:=os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
 if port=="" {port="443"}
 number,err:=strconv.Atoi(port)
 if err!=nil||number<1||number>65535 || host=="" {return Plan{},errors.New("invalid Kubernetes API address")}
 // A service-account and validated CA must exist before exposing the button.
 if _,err:=os.Stat(serviceAccountDir+"/token");err!=nil{return Plan{},fmt.Errorf("Kubernetes token unavailable: %w",err)}
 roots,err:=loadKubernetesCA(serviceAccountDir+"/ca.crt")
 if err!=nil{return Plan{},err}
 client:=&http.Client{
  Transport:&http.Transport{TLSClientConfig:&tls.Config{MinVersion:tls.VersionTLS12,RootCAs:roots},Proxy:nil},
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
  Timeout:12*time.Second,
 }
 cfg:=&KubernetesConfig{
  ServerURL:"https://"+net.JoinHostPort(host,port),Namespace:namespace,
  Deployment:name,TokenPath:serviceAccountDir+"/token",
  CAPath:serviceAccountDir+"/ca.crt",Client:client,
 }
 return Plan{Manager:"kubernetes",kubernetes:cfg},nil
}

func validKubernetesName(s string)bool{
 if s==""||len(s)>63||s[0]<'a'||s[0]>'z' && (s[0]<'0'||s[0]>'9') {return false}
 if last:=s[len(s)-1]; !(last>='a'&&last<='z'||last>='0'&&last<='9'){return false}
 for _,c:=range s{
  if c>='a'&&c<='z'||c>='0'&&c<='9'||c=='-' {continue}
  return false
 }
 return true
}

func loadKubernetesCA(filename string)(*x509.CertPool,error){
 ca,err:=os.ReadFile(filename)
 if err!=nil{return nil,fmt.Errorf("Kubernetes CA unavailable: %w",err)}
 pool:=x509.NewCertPool()
 if !pool.AppendCertsFromPEM(ca){return nil,errors.New("invalid Kubernetes API CA")}
 return pool,nil
}

// Restart performs a Deployment rollout restart by updating a single
// pod-template annotation. RBAC must grant patch only on the named deployment.
// No shell, PID, pod deletion, or user-supplied API path is involved.
func (k *KubernetesConfig) Restart(ctx context.Context)error{
 if k==nil||k.Client==nil||!validKubernetesName(k.Namespace)||!validKubernetesName(k.Deployment) {
  return errors.New("Kubernetes restart adapter unavailable")
 }
 token,err:=os.ReadFile(k.TokenPath)
 if err!=nil{return fmt.Errorf("Kubernetes token read: %w",err)}
 credential:=strings.TrimSpace(string(token))
 if credential==""{return errors.New("empty Kubernetes service-account token")}
 // Exactly the workload's pod-template metadata, not the deployment spec.
 patch:=map[string]any{"spec":map[string]any{"template":map[string]any{
  "metadata":map[string]any{"annotations":map[string]string{
   "sparky.io/restarted-at":time.Now().UTC().Format(time.RFC3339Nano),
  }},
 }}}
 payload,err:=json.Marshal(patch)
 if err!=nil{return err}
 u,err:=url.Parse(k.ServerURL)
 if err!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||u.Path!=""{
  return errors.New("invalid Kubernetes API endpoint")
 }
 u.Path=path.Join("/apis/apps/v1/namespaces",k.Namespace,"deployments",k.Deployment)
 req,err:=http.NewRequestWithContext(ctx,http.MethodPatch,u.String(),bytes.NewReader(payload))
 if err!=nil{return err}
 req.Header.Set("Authorization","Bearer "+credential)
 req.Header.Set("Content-Type","application/merge-patch+json")
 req.Header.Set("Accept","application/json")
 response,err:=k.Client.Do(req)
 if err!=nil{return err}
 defer response.Body.Close()
 // Always consume a small bounded amount and close for connection reuse;
 // do not echo API errors or credentials into the web UI.
 _,_=io.Copy(io.Discard,io.LimitReader(response.Body,4096))
 if response.StatusCode<200 || response.StatusCode>=300 {
  return fmt.Errorf("Kubernetes deployment restart rejected (HTTP %d)",response.StatusCode)
 }
 return nil
}
