package webconfig

import (
 "context"
 "errors"
 "fmt"
 "io"
 "math"
 "net"
 "net/http"
 "net/url"
 "strconv"
 "strings"
 "time"
)

// ValidateMetricsURL permits only the gateway's local read-only endpoint.
// No external URLs, DNS names, credentials, query strings, or redirects.
func ValidateMetricsURL(raw string) error {
 if raw=="" {return nil}
 u,err:=url.Parse(raw)
 if err!=nil||u.Scheme!="http"||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||u.Opaque!=""||u.Path!="/metrics"{
  return errors.New("metrics URL must be a local http://IP:port/metrics endpoint")
 }
 host,port,err:=net.SplitHostPort(u.Host)
 if err!=nil || port==""{return errors.New("metrics URL requires a literal loopback IP and port")}
 ip:=net.ParseIP(host)
 if ip==nil||!ip.IsLoopback(){return errors.New("metrics endpoint must use a loopback IP")}
 if u.RawPath!="" {return errors.New("metrics URL may not contain encoded paths")}
 return nil
}

type runtimeStatus struct {
 Enabled bool
 Available bool
 Ready bool
 Uptime string
 Delivered uint64
 Dropped uint64
}

func (s *Server) runtimeStatus(ctx context.Context) runtimeStatus {
 result:=runtimeStatus{Enabled:s.MetricsURL!=""}
 if !result.Enabled || ValidateMetricsURL(s.MetricsURL)!=nil{return result}
 requestCtx,cancel:=context.WithTimeout(ctx,400*time.Millisecond)
 defer cancel()
 req,err:=http.NewRequestWithContext(requestCtx,http.MethodGet,s.MetricsURL,nil)
 if err!=nil{return result}
 client:=s.MetricsClient
 if client==nil{
  client=&http.Client{
   Timeout:400*time.Millisecond,
   Transport:&http.Transport{Proxy:nil,DisableKeepAlives:true},
   CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
  }
 }
 resp,err:=client.Do(req)
 if err!=nil{return result}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return result}
 body,err:=io.ReadAll(io.LimitReader(resp.Body,8193))
 if err!=nil||len(body)>8192{return result}
 values:=make(map[string]string,4)
 for _,line:=range strings.Split(string(body),"\n"){
  if strings.HasPrefix(line,"#")||line==""{continue}
  fields:=strings.Fields(line)
  if len(fields)!=2{continue}
  switch fields[0] {
  case "sparky_ready","sparky_uptime_seconds","sparky_pubsub_deliveries_total","sparky_pubsub_dropped_deliveries_total":
   if _,exists:=values[fields[0]];exists{return result}
   values[fields[0]]=fields[1]
  }
 }
 for _,name:=range []string{"sparky_ready","sparky_uptime_seconds","sparky_pubsub_deliveries_total","sparky_pubsub_dropped_deliveries_total"}{
  if _,ok:=values[name];!ok{return result}
 }
 if values["sparky_ready"]!="0"&&values["sparky_ready"]!="1"{return result}
 seconds,err:=strconv.ParseFloat(values["sparky_uptime_seconds"],64)
 if err!=nil||math.IsNaN(seconds)||math.IsInf(seconds,0)||seconds<0||seconds>1e12{return result}
 delivered,err:=strconv.ParseUint(values["sparky_pubsub_deliveries_total"],10,64)
 if err!=nil{return result}
 dropped,err:=strconv.ParseUint(values["sparky_pubsub_dropped_deliveries_total"],10,64)
 if err!=nil{return result}
 result.Ready=values["sparky_ready"]=="1"
 result.Uptime=fmt.Sprintf("%.0fs",seconds)
 result.Delivered=delivered
 result.Dropped=dropped
 result.Available=true
 return result
}
