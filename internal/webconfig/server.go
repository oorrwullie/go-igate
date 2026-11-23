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
 // PublicOrigin is the browser-visible HTTPS origin (for example,
 // https://sparky.example.org). Forwarded-* headers are never trusted.
 PublicOrigin string
 // TrustedProxyIP is Routy's source IP. In LAN proxy mode only that source
 // may send requests to the plaintext listener with PublicOrigin's Host.
 TrustedProxyIP string
 // AllowLANHTTP is an explicit acknowledgment that Routy-to-Sparky traffic
 // is unencrypted. A firewall must restrict the listener to TrustedProxyIP.
 AllowLANHTTP bool
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

// allowedRequest accepts direct TLS, local loopback requests, or the exact
// public Host from an explicitly trusted proxy source IP. Forwarded-* headers
// are never trusted for authorization.
func (s *Server) allowedRequest(r *http.Request) bool {
 if r.TLS!=nil {return true}
 sourceHost,_,err:=net.SplitHostPort(r.RemoteAddr)
 source:=net.ParseIP(sourceHost)
 if err!=nil || source==nil {return false}
 // For a loopback-only deployment, allow local addresses and local Host
 // headers. When an operator opts into LAN HTTP, do not accept arbitrary
 // plaintext requests even if they spoof localhost in Host.
 if source.IsLoopback() && localHost(r.Host) {return true}
 if s==nil || s.PublicOrigin=="" {return false}
 u,err:=parsePublicOrigin(s.PublicOrigin)
 if err!=nil || r.Host!=u.Host{return false}
 if s.TrustedProxyIP=="" {return source.IsLoopback()}
 proxyIP:=net.ParseIP(s.TrustedProxyIP)
 return proxyIP!=nil && source.Equal(proxyIP)
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
// HTTP Basic authentication is always required. Operator-enabled LAN proxy
// mode relies on TLS termination at Routy and firewall isolation of the hop.
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

// ListenAndServe defaults to loopback-only HTTP. Non-loopback plaintext
// requires an explicit, restricted LAN proxy configuration.
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
 if s.PublicOrigin!="" && secure {return errors.New("public origin proxy mode cannot be combined with direct TLS")}
 if s.AllowLANHTTP {
  if ip.IsLoopback() {return errors.New("LAN HTTP opt-in requires a non-loopback bind address")}
  if s.PublicOrigin=="" {return errors.New("LAN HTTP proxy requires a public HTTPS origin")}
  trusted:=net.ParseIP(s.TrustedProxyIP)
  if trusted==nil || trusted.IsUnspecified() || trusted.IsMulticast() {
   return errors.New("LAN HTTP proxy requires a valid trusted proxy IP")
  }
  if secure {return errors.New("LAN HTTP opt-in cannot be combined with direct TLS")}
 } else if !ip.IsLoopback() && !secure {
  return errors.New("non-loopback HTTP requires explicit LAN proxy opt-in or direct TLS")
 }
 if s.TrustedProxyIP!="" && net.ParseIP(s.TrustedProxyIP)==nil {
  return errors.New("trusted proxy IP must be a literal IP address")
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
