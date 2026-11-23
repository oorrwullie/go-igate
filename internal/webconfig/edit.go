package webconfig

import (
 "crypto/rand"
 "crypto/subtle"
 "encoding/hex"
 "errors"
 "html/template"
 "net/http"
 "strings"

 "github.com/oorrwullie/sparky/internal/config"
 "github.com/oorrwullie/sparky/internal/configeditor"
 "gopkg.in/yaml.v3"
)

var editTemplate=template.Must(template.New("edit").Parse(editHTML))
type editValues struct{ Callsign string; IGate, Digipeater, SDR, Transmitter, MQTT, ADSB bool }
type editData struct{ Values editValues; Revision, Token string; Saved bool }

// A single random CSRF token belongs to each independently running editor.
// Basic authentication is required for both reading and saving the edit form.
func csrfToken(s *Server) ([]byte,error) {
 s.tokenOnce.Do(func(){
  token:=make([]byte,32)
  if _,err:=rand.Read(token);err!=nil{s.tokenErr=err;return}
  s.token=token
 })
 return s.token,s.tokenErr
}
func (s *Server) serveEdit(w http.ResponseWriter,r *http.Request) {
 token,err:=csrfToken(s)
 if err!=nil{http.Error(w,"Editor unavailable",503);return}
 if r.Method==http.MethodPost {s.saveEdit(w,r,token);return}
 if r.Method!=http.MethodGet {w.Header().Set("Allow","GET, POST");http.Error(w,"Method not allowed",405);return}
 bytes,revision,err:=s.Store.Load()
 if err!=nil{http.Error(w,"Configuration unavailable",503);return}
 cfg,err:=config.ParseConfigBytes(bytes)
 if err!=nil{http.Error(w,"Configuration invalid",503);return}
 data:=editData{
  Revision:revision,Token:hex.EncodeToString(token),Saved:r.URL.Query().Get("saved")=="1",
  Values:editValues{cfg.StationCallsign,cfg.IGate.Enabled,cfg.DigipeaterEnabled,cfg.Sdr.Enabled,cfg.Transmitter.Enabled,cfg.OpenWebRXMQTT.Enabled,cfg.MapFlights.Enabled},
 }
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 _=editTemplate.Execute(w,data)
}
func (s *Server) saveEdit(w http.ResponseWriter,r *http.Request,token []byte) {
 expectedOrigin:=s.expectedOrigin(r)
 if r.Header.Get("Origin")!=expectedOrigin {http.Error(w,"Invalid request origin",403);return}
 r.Body=http.MaxBytesReader(w,r.Body,configeditor.MaxDocumentSize+4096)
 if err:=r.ParseForm();err!=nil{http.Error(w,"Invalid form",400);return}
 provided,err:=hex.DecodeString(r.PostForm.Get("csrf"))
 if err!=nil || subtle.ConstantTimeCompare(provided,token)!=1 {http.Error(w,"Invalid CSRF token",403);return}
 revision:=r.PostForm.Get("revision")
 if len(revision)!=64{http.Error(w,"Invalid revision",400);return}
 original,current,err:=s.Store.Load()
 if err!=nil{http.Error(w,"Configuration unavailable",503);return}
 if current!=revision{http.Error(w,"Configuration changed; reload to avoid losing edits",409);return}
 v:=editValues{Callsign:strings.TrimSpace(r.PostForm.Get("callsign")),
  IGate:r.PostForm.Has("igate"),Digipeater:r.PostForm.Has("digipeater"),SDR:r.PostForm.Has("sdr"),
  Transmitter:r.PostForm.Has("transmitter"),MQTT:r.PostForm.Has("mqtt"),ADSB:r.PostForm.Has("adsb")}
 candidate,err:=patchEditable(original,v)
 if err!=nil{http.Error(w,"Invalid configuration document",400);return}
 _,err=s.Store.Save(candidate,revision)
 if errors.Is(err,configeditor.ErrConflict){http.Error(w,"Configuration changed; reload",409);return}
 if errors.Is(err,configeditor.ErrDurability){http.Error(w,"Configuration saved, but filesystem durability is unconfirmed; inspect before restarting",500);return}
 if err!=nil{http.Error(w,"Configuration rejected or save failed; check server logs",400);return}
 http.Redirect(w,r,"/edit?saved=1",http.StatusSeeOther)
}
func patchEditable(original []byte,v editValues)([]byte,error){
 var root yaml.Node
 if err:=yaml.Unmarshal(original,&root);err!=nil{return nil,err}
 if len(root.Content)!=1 || root.Content[0].Kind!=yaml.MappingNode{return nil,errors.New("expected YAML mapping")}
 options:=[]struct{path []string; value string; tag string}{
  {[]string{"station-callsign"},v.Callsign,"!!str"},
  {[]string{"igate","enabled"},asBool(v.IGate),"!!bool"},
  {[]string{"enable-digipeater"},asBool(v.Digipeater),"!!bool"},
  {[]string{"sdr","enabled"},asBool(v.SDR),"!!bool"},
  {[]string{"transmitter","enabled"},asBool(v.Transmitter),"!!bool"},
  {[]string{"openwebrx-mqtt","enabled"},asBool(v.MQTT),"!!bool"},
  {[]string{"map-flights","enabled"},asBool(v.ADSB),"!!bool"},
 }
 for _,o:=range options {if err:=setEditable(root.Content[0],o.path,o.value,o.tag);err!=nil{return nil,err}}
 return yaml.Marshal(&root)
}
func asBool(v bool)string{if v{return "true"};return "false"}
func setEditable(node *yaml.Node,path []string,value,tag string)error{
 for i,k:=range path{
  if node.Kind!=yaml.MappingNode{return errors.New("expected YAML mapping")}
  var child *yaml.Node
  for j:=0;j+1<len(node.Content);j+=2{
   if node.Content[j].Value==k {child=node.Content[j+1];break}
  }
  if child==nil{
   child=&yaml.Node{}
   node.Content=append(node.Content,&yaml.Node{Kind:yaml.ScalarNode,Tag:"!!str",Value:k},child)
  }
  if i==len(path)-1{
   if child.Kind!=0 && child.Kind!=yaml.ScalarNode{return errors.New("editable field must be scalar")}
   child.Kind=yaml.ScalarNode;child.Tag=tag;child.Value=value;child.Style=0
  }else{
   if child.Kind==0 {child.Kind=yaml.MappingNode;child.Tag="!!map"}
   node=child
  }
 }
 return nil
}
