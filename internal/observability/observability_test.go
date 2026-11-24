package observability

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "sync"
 "testing"
)

func TestProbesLifecycleAndMethods(t *testing.T) {
 r:=New()
 h:=r.Handler()
 check:=func(path string,want int){
  t.Helper()
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,httptest.NewRequest("GET",path,nil))
  if w.Code!=want{t.Fatalf("%s: %d want %d",path,w.Code,want)}
 }
 check("/healthz",200)
 check("/readyz",503)
 r.Ready()
 check("/readyz",200)
 r.Stopping()
 check("/readyz",503)
 check("/healthz",503)
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest("POST","/metrics",nil))
 if w.Code!=http.StatusMethodNotAllowed||w.Header().Get("Allow")!="GET, HEAD"{t.Fatal("write method accepted")}
}

func TestMetricsStableAndUnlabeled(t *testing.T) {
 r:=New()
 r.Snapshot=func()(uint64,uint64){return 71,3}
 r.Ready()
 w:=httptest.NewRecorder()
 r.Handler().ServeHTTP(w,httptest.NewRequest("GET","/metrics",nil))
 if w.Code!=200{t.Fatalf("code %d",w.Code)}
 body:=w.Body.String()
 for _,metric:=range []string{
  "sparky_ready 1","sparky_pubsub_deliveries_total 71",
  "sparky_pubsub_dropped_deliveries_total 3","sparky_uptime_seconds ",
 }{
  if !strings.Contains(body,metric){t.Fatalf("missing %q in %q",metric,body)}
 }
 if strings.Contains(body,"{"){t.Fatal("unexpected dynamic labels")}
 head:=httptest.NewRecorder()
 r.Handler().ServeHTTP(head,httptest.NewRequest("HEAD","/metrics",nil))
 if head.Code!=200||head.Body.Len()!=0{t.Fatal("HEAD request returned body")}
}

func TestConcurrentLifecycleAndScrapes(t *testing.T) {
 r:=New()
 r.Snapshot=func()(uint64,uint64){return 17,2}
 r.Ready()
 var wg sync.WaitGroup
 for i:=0;i<4;i++ {
  wg.Add(1)
  go func(){
   defer wg.Done()
   for j:=0;j<100;j++{
    w:=httptest.NewRecorder()
    r.Handler().ServeHTTP(w,httptest.NewRequest("GET","/metrics",nil))
    if w.Code!=200{t.Errorf("scrape failed: %d",w.Code)}
   }
  }()
 }
 r.Stopping()
 wg.Wait()
}

// Detect regressions on the read side; the receive side has a separate
// benchmark in internal/pubsub.
func BenchmarkMetricsScrape(b *testing.B) {
 r:=New()
 r.Ready()
 r.Snapshot=func()(uint64,uint64){return 100,5}
 h:=r.Handler()
 req:=httptest.NewRequest("GET","/metrics",nil)
 b.ReportAllocs()
 for i:=0;i<b.N;i++{h.ServeHTTP(httptest.NewRecorder(),req)}
}
