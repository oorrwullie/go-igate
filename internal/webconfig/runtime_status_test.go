package webconfig

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "sync"
 "context"
)

const goodMetrics = "# HELP sparky_ready Ready\n# TYPE sparky_ready gauge\nsparky_ready 1\nsparky_uptime_seconds 122.500\nsparky_pubsub_deliveries_total 24\nsparky_pubsub_dropped_deliveries_total 2\n"

func TestMetricsURLValidation(t *testing.T){
 for _,v:=range []string{"","http://127.0.0.1:9091/metrics","http://[::1]:9091/metrics"}{
  if err:=ValidateMetricsURL(v);err!=nil{t.Errorf("rejected %q: %v",v,err)}
 }
 for _,v:=range []string{
  "https://127.0.0.1:9091/metrics",
  "http://example.com:9091/metrics",
  "http://localhost:9091/metrics",
  "http://127.0.0.2:9091/status",
  "http://127.0.0.1:9091/metrics?token=secret",
  "http://admin@127.0.0.1:9091/metrics",
  "http://127.0.0.1:9091/%6detrics",
  "http://169.254.169.254:80/metrics",
 }{
  if err:=ValidateMetricsURL(v);err==nil{t.Errorf("accepted unsafe metrics endpoint: %q",v)}
 }
}

func TestRuntimeStatusSnapshotOnOverview(t *testing.T){
 upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/metrics" {t.Errorf("unexpected metrics path %s",r.URL.Path)}
  _,_=w.Write([]byte(goodMetrics))
 }))
 defer upstream.Close()
 s:=testServer(t)
 s.MetricsURL=upstream.URL+"/metrics"
 req:=localTestRequest("GET","http://127.0.0.1:8080/",nil)
 req.SetBasicAuth("admin","this-is-a-long-test-password")
 w:=httptest.NewRecorder()
 s.ServeHTTP(w,req)
 if w.Code!=200{t.Fatalf("status %d",w.Code)}
 for _,v:=range []string{"● READY","122s","24","2"}{
  if !strings.Contains(w.Body.String(),v){t.Errorf("missing runtime value %q",v)}
 }
}

func TestRuntimeStatusFailsClosedForBadMetrics(t *testing.T){
 for _,body:=range []string{"", "sparky_ready 1\n",strings.Replace(goodMetrics,"sparky_ready 1","sparky_ready 2",1),goodMetrics+"sparky_ready 0\n",strings.Replace(goodMetrics,"122.500","NaN",1),strings.Replace(goodMetrics,"122.500","+Inf",1),strings.Replace(goodMetrics,"122.500","-Inf",1)}{
  t.Run(body,func(t *testing.T){
   upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){_,_=w.Write([]byte(body))}))
   defer upstream.Close()
   s:=testServer(t)
   s.MetricsURL=upstream.URL+"/metrics"
   req:=localTestRequest("GET","http://127.0.0.1:8080/",nil)
   req.SetBasicAuth("admin","this-is-a-long-test-password")
   w:=httptest.NewRecorder();s.ServeHTTP(w,req)
   if w.Code!=200||!strings.Contains(w.Body.String(),"runtime status unavailable") {
    t.Fatalf("bad metrics not handled gracefully: %d",w.Code)
   }
  })
 }
}

func TestRuntimeStatusNotFetchedForUnauthenticatedUser(t *testing.T){
 var calls int
 upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){calls++;_,_=w.Write([]byte(goodMetrics))}))
 defer upstream.Close()
 s:=testServer(t);s.MetricsURL=upstream.URL+"/metrics"
 w:=httptest.NewRecorder();s.ServeHTTP(w,localTestRequest("GET","http://127.0.0.1:8080/",nil))
 if w.Code!=401||calls!=0{t.Fatal("unauthenticated request accessed gateway metrics")}
}

func TestRuntimeStatusRepeatedScrapesDoNotRetainIdleConnections(t *testing.T){
 var mu sync.Mutex
 var sources []string
 var requestedClose []bool
 upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  mu.Lock()
  sources=append(sources,r.RemoteAddr)
  requestedClose=append(requestedClose,r.Close)
  mu.Unlock()
  _,_=w.Write([]byte(goodMetrics))
 }))
 defer upstream.Close()
 s:=testServer(t)
 s.MetricsURL=upstream.URL+"/metrics"
 for i:=0;i<2;i++{
  status:=s.runtimeStatus(context.Background())
  if !status.Available{t.Fatal("expected valid metrics")}
 }
 mu.Lock()
 defer mu.Unlock()
 if len(sources)!=2{t.Fatalf("requests=%d, expected 2",len(sources))}
 if sources[0]==sources[1]{t.Errorf("requests unexpectedly reused an idle connection: %v",sources)}
 for i,closed:=range requestedClose{
  if !closed{t.Errorf("request %d retained keep-alive",i)}
 }
}
