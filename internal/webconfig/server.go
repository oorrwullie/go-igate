// Package webconfig serves the first, read-only Sparky configuration interface.
package webconfig

import (
 "errors"
 "fmt"
 "html/template"
 "net"
 "net/http"
 "strings"
 "time"

 "github.com/oorrwullie/sparky/internal/config"
 "github.com/oorrwullie/sparky/internal/configeditor"
)

type Server struct { Store *configeditor.Store }

type view struct {
 Callsign string
 SDR bool
 SDRFrequency string
 IGate bool
 APRSIS bool
 Digipeater bool
 Transmitter bool
 MQTT bool
 MQTTTopic string
 ADSB bool
}

var page = template.Must(template.New("page").Parse(pageHTML))

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
 w.Header().Set("Cache-Control", "no-store")
 w.Header().Set("X-Content-Type-Options", "nosniff")
 w.Header().Set("X-Frame-Options", "DENY")
 w.Header().Set("Referrer-Policy", "no-referrer")
 w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
 if !localHost(r.Host) {
  http.Error(w,"Host not allowed",http.StatusForbidden)
  return
 }
 if r.URL.Path != "/" {
  http.NotFound(w,r)
  return
 }
 if r.Method != http.MethodGet && r.Method != http.MethodHead {
  w.Header().Set("Allow","GET, HEAD")
  http.Error(w,"Method not allowed",http.StatusMethodNotAllowed)
  return
 }
 if s == nil || s.Store == nil {
  http.Error(w,"Configuration unavailable",http.StatusServiceUnavailable)
  return
 }
 raw,_,err:=s.Store.Load()
 if err!=nil {
  http.Error(w,"Configuration unavailable",http.StatusServiceUnavailable)
  return
 }
 cfg,err:=config.ParseConfigBytes(raw)
 if err!=nil {
  http.Error(w,"Configuration invalid; inspect it locally",http.StatusServiceUnavailable)
  return
 }
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 if r.Method == http.MethodHead {return}
 data:=view{
  Callsign:cfg.StationCallsign, SDR:cfg.Sdr.Enabled,
  SDRFrequency:cfg.Sdr.Frequency,IGate:cfg.IGate.Enabled,
  APRSIS:cfg.IGate.Aprsis.Enabled,Digipeater:cfg.DigipeaterEnabled,
  Transmitter:cfg.Transmitter.Enabled,MQTT:cfg.OpenWebRXMQTT.Enabled,
  MQTTTopic:cfg.OpenWebRXMQTT.Topic,ADSB:cfg.MapFlights.Enabled,
 }
 if err:=page.Execute(w,data);err!=nil {return}
}

// localHost prevents cross-site DNS rebinding requests to a loopback listener.
// In particular, arbitrary external hostnames must not be accepted.
func localHost(hostport string) bool {
 host:=hostport
 if h,_,err:=net.SplitHostPort(hostport);err==nil {host=h}
 host=strings.Trim(host,"[]")
 if strings.EqualFold(host,"localhost") || strings.EqualFold(host,"localhost.") {return true}
 ip:=net.ParseIP(host)
 return ip!=nil && ip.IsLoopback()
}

// ListenAndServe starts only on a loopback address. It never binds a LAN
// interface, regardless of a caller's requested address.
func (s *Server) ListenAndServe(addr string) error {
 if addr=="" {addr="127.0.0.1:8080"}
 host,_,err:=net.SplitHostPort(addr)
 if err!=nil || !localHost(host) {
  return fmt.Errorf("web config listener must use an explicit loopback IP address: %q",addr)
 }
 // Require literal loopback IP rather than a DNS name to avoid binding surprises.
 ip:=net.ParseIP(host)
 if ip==nil || !ip.IsLoopback() {return errors.New("listener requires a literal loopback IP")}
 listener,err:=net.Listen("tcp",addr)
 if err!=nil {return err}
 httpServer:=&http.Server{
  Handler:s,
  ReadHeaderTimeout:5*time.Second,
  ReadTimeout:10*time.Second,
  WriteTimeout:10*time.Second,
  IdleTimeout:30*time.Second,
  MaxHeaderBytes:1<<16,
 }
 return httpServer.Serve(listener)
}
