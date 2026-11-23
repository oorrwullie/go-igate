// Package webconfig serves Sparky's authenticated configuration interface.
package webconfig

import (
 "crypto/sha256"
 "crypto/subtle"
 "crypto/tls"
 "errors"
 "fmt"
 "html/template"
 "net"
 "net/http"
 "net/url"
 "os"
 "strings"
 "sync"
 "time"

 "github.com/oorrwullie/sparky/internal/config"
 "github.com/oorrwullie/sparky/internal/configeditor"
)

type Server struct {
 Store *configeditor.Store
 // PublicOrigin opts into HTTPS termination by a trusted same-host proxy.
 // It must be an exact https://origin, without a path.
 PublicOrigin string
 tokenOnce sync.Once
 token []byte
 tokenErr error
}

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
 if !s.allowedRequest(r) {
  http.Error(w,"Host not allowed",http.StatusForbidden)
  return
 }
 if !s.authorized(w,r) {return}
 if r.URL.Path == "/edit" {
  w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
  if s == nil || s.Store == nil {http.Error(w,"Configuration unavailable",503);return}
  s.serveEdit(w,r)
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

// allowedRequest accepts direct loopback requests, direct TLS requests and
// exactly one configured HTTPS origin from a same-host loopback proxy.
// Forwarded-* headers are never trusted for authorization.
func (s *Server) allowedRequest(r *http.Request) bool {
 if r.TLS!=nil { return true }
 if localHost(r.Host) {return true}
 if s==nil || s.PublicOrigin=="" {return false}
 u,err:=parsePublicOrigin(s.PublicOrigin)
 if err!=nil || r.Host!=u.Host{return false}
 host,_,err:=net.SplitHostPort(r.RemoteAddr)
 return err==nil && func()bool{ip:=net.ParseIP(host);return ip!=nil&&ip.IsLoopback()}()
}

func parsePublicOrigin(origin string) (*url.URL,error) {
 u,err:=url.Parse(origin)
 if err!=nil || u.Scheme!="https" || u.Host=="" || u.Hostname()=="" ||
  u.User!=nil || u.Path!="" || u.RawQuery!="" || u.Fragment!="" || u.Opaque!="" {
  return nil, errors.New("public origin must be a single https://hostname[:port] without a path")
 }
 return u,nil
}

// expectedOrigin uses a configured public URL only for a trusted proxy
// request. Non-proxied requests use the actual request scheme and Host.
func (s *Server) expectedOrigin(r *http.Request) string {
 if r.TLS==nil && s!=nil && s.PublicOrigin!="" {
  if u,err:=parsePublicOrigin(s.PublicOrigin);err==nil && r.Host==u.Host {
   return u.String()
  }
 }
 scheme:="http"
 if r.TLS!=nil{scheme="https"}
 return scheme+"://"+r.Host
}

// authorized protects every route, including the overview and error pages.
// Plain HTTP is permitted only on the loopback listener. Remote access
// requires HTTPS configured with an explicit certificate and private key.
func (s *Server) authorized(w http.ResponseWriter,r *http.Request) bool {
 password:=os.Getenv("SPARKY_CONFIG_PASSWORD")
 if len(password)<16 {
  http.Error(w,"Configuration interface disabled: set SPARKY_CONFIG_PASSWORD (16+ characters)",http.StatusServiceUnavailable)
  return false
 }
 user,given,ok:=r.BasicAuth()
 expectedUser:=sha256.Sum256([]byte("admin"))
 actualUser:=sha256.Sum256([]byte(user))
 expectedPass:=sha256.Sum256([]byte(password))
 actualPass:=sha256.Sum256([]byte(given))
 if !ok || subtle.ConstantTimeCompare(actualUser[:],expectedUser[:])!=1 || subtle.ConstantTimeCompare(actualPass[:],expectedPass[:])!=1 {
  w.Header().Set("WWW-Authenticate",`Basic realm="Sparky Configuration", charset="UTF-8"`)
  http.Error(w,"Authentication required",http.StatusUnauthorized)
  return false
 }
 return true
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

// ListenAndServe keeps the original safe default: plaintext HTTP bound to an
// explicit loopback IP address. LAN exposure requires ListenAndServeTLS.
func (s *Server) ListenAndServe(addr string) error {
 return s.listen(addr,"","")
}

// ListenAndServeTLS enables explicit HTTPS on a chosen interface. Both TLS
// credentials must be supplied, including for localhost, and the browser must
// trust a certificate valid for the hostname or IP used.
func (s *Server) ListenAndServeTLS(addr, certFile, keyFile string) error {
 if certFile=="" || keyFile=="" {return errors.New("HTTPS requires both TLS certificate and private key")}
 return s.listen(addr,certFile,keyFile)
}

func (s *Server) listen(addr,certFile,keyFile string) error {
 if addr=="" {addr="127.0.0.1:8080"}
 if s.PublicOrigin!="" {
  if _,err:=parsePublicOrigin(s.PublicOrigin);err!=nil{return err}
 }
 host,_,err:=net.SplitHostPort(addr)
 if err!=nil {return fmt.Errorf("invalid web configuration listener %q: %w",addr,err)}
 ip:=net.ParseIP(host)
 if ip==nil {return errors.New("listener requires a literal IP address")}
 secure:=certFile!="" && keyFile!=""
 if s.PublicOrigin!="" && (!ip.IsLoopback() || secure) {return errors.New("public origin proxy mode requires a loopback HTTP listener with no direct TLS") }
 if !ip.IsLoopback() && !secure {
  return errors.New("non-loopback web configuration listener requires HTTPS certificate and key")
 }
 if (certFile=="") != (keyFile=="") {return errors.New("incomplete TLS configuration")}
 var cert tls.Certificate
 if secure {
  cert,err=tls.LoadX509KeyPair(certFile,keyFile)
  if err!=nil {return fmt.Errorf("load HTTPS certificate: %w",err)}
 }
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
 if secure {
  httpServer.TLSConfig=&tls.Config{MinVersion:tls.VersionTLS12,Certificates:[]tls.Certificate{cert}}
  return httpServer.ServeTLS(listener,"","")
 }
 return httpServer.Serve(listener)
}
