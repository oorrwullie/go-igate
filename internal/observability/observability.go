// Package observability exposes bounded, read-only local health and metrics.
// It never performs I/O on packet-processing paths.
package observability

import (
 "fmt"
 "net/http"
 "strconv"
 "strings"
 "sync/atomic"
 "time"
)

type Registry struct {
 started time.Time
 ready atomic.Bool
 stopping atomic.Bool
 // Snapshot must perform only bounded, nonblocking atomic loads.
 Snapshot func() (published, dropped uint64)
 // OperationalSnapshot is attached before serving and reads atomic counters and local connection state; no network I/O.
 OperationalSnapshot func() Operations
}

func New() *Registry { return &Registry{started:time.Now()} }
func (r *Registry) Ready() { r.ready.Store(true) }
func (r *Registry) Stopping() { r.stopping.Store(true);r.ready.Store(false) }

func (r *Registry) Handler() http.Handler {
 mux:=http.NewServeMux()
 mux.HandleFunc("/healthz",func(w http.ResponseWriter,req *http.Request){
  if !readMethod(w,req){return}
  status:=http.StatusOK
  // Liveness measures process responsiveness; external dependencies are not polled.
  if r.stopping.Load(){status=http.StatusServiceUnavailable}
  statusText(w,req,status)
 })
 mux.HandleFunc("/readyz",func(w http.ResponseWriter,req *http.Request){
  if !readMethod(w,req){return}
  status:=http.StatusServiceUnavailable
  if r.ready.Load() && !r.stopping.Load(){status=http.StatusOK}
  statusText(w,req,status)
 })
 mux.HandleFunc("/metrics",func(w http.ResponseWriter,req *http.Request){
  if !readMethod(w,req){return}
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("Content-Type","text/plain; version=0.0.4; charset=utf-8")
  if req.Method==http.MethodHead{return}
  published,dropped:=uint64(0),uint64(0)
  if r.Snapshot!=nil{published,dropped=r.Snapshot()}
  ready:=0
  if r.ready.Load()&&!r.stopping.Load(){ready=1}
  var b strings.Builder
  b.Grow(512)
  b.WriteString("# HELP sparky_uptime_seconds Process uptime in seconds.\n# TYPE sparky_uptime_seconds gauge\nsparky_uptime_seconds ")
  b.WriteString(strconv.FormatFloat(time.Since(r.started).Seconds(),'f',3,64))
  b.WriteString("\n# HELP sparky_ready Whether the gateway has completed startup and is not stopping.\n# TYPE sparky_ready gauge\nsparky_ready ")
  b.WriteString(strconv.Itoa(ready))
  b.WriteString("\n# HELP sparky_pubsub_deliveries_total Packet deliveries admitted into subscriber queues.\n# TYPE sparky_pubsub_deliveries_total counter\nsparky_pubsub_deliveries_total ")
  b.WriteString(strconv.FormatUint(published,10))
  b.WriteString("\n# HELP sparky_pubsub_dropped_deliveries_total Packet deliveries dropped for full subscriber queues.\n# TYPE sparky_pubsub_dropped_deliveries_total counter\nsparky_pubsub_dropped_deliveries_total ")
  b.WriteString(strconv.FormatUint(dropped,10))
  b.WriteByte('\n')
  if r.OperationalSnapshot != nil { r.OperationalSnapshot().writeMetrics(&b) }
  _,_=w.Write([]byte(b.String()))
 })
 return mux
}
func readMethod(w http.ResponseWriter,r *http.Request)bool {
 if r.Method!=http.MethodGet && r.Method!=http.MethodHead {
  w.Header().Set("Allow","GET, HEAD")
  http.Error(w,"Method not allowed",http.StatusMethodNotAllowed)
  return false
 }
 return true
}
func statusText(w http.ResponseWriter,r *http.Request,status int){
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("Content-Type","text/plain; charset=utf-8")
 w.WriteHeader(status)
 if r.Method==http.MethodGet {_,_=fmt.Fprintln(w,http.StatusText(status))}
}
