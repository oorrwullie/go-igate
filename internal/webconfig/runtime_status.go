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
 Operations []operationMetric
}

type operationMetric struct { Label, Value string }

// Optional operational metrics are deliberately ordered and allowlisted.
var operationalFields = []struct { name, label string; kind byte }{
 {"sparky_aprs_input_frames_total", "APRS input frames", 'c'},
 {"sparky_aprs_duplicates_suppressed_total", "APRS duplicates suppressed", 'c'},
 {"sparky_aprs_last_input_timestamp_seconds", "Last APRS input (UTC)", 't'},
 {"sparky_aprsis_connected", "APRS-IS connection", 'b'},
 {"sparky_aprsis_rf_forward_writes_total", "APRS-IS forward writes", 'c'},
 {"sparky_aprsis_rf_forward_errors_total", "APRS-IS forward errors", 'c'},
 {"sparky_tx_dispatched_frames_total", "TX frames dispatched (not confirmed RF)", 'c'},
 {"sparky_adsb_input_reports_total", "ADS-B input reports", 'c'},
 {"sparky_adsb_publish_completed_total", "ADS-B publish completions", 'c'},
 {"sparky_adsb_publish_errors_total", "ADS-B publish errors", 'c'},
 {"sparky_adsb_rejected_total", "ADS-B rejected reports", 'c'},
 {"sparky_adsb_offline_dropped_total", "ADS-B offline drops", 'c'},
 {"sparky_adsb_queue_dropped_total", "ADS-B queue drops", 'c'},
 {"sparky_mqtt_input_connected", "External MQTT connection", 'b'},
 {"sparky_decoder_running", "Local decoder reader", 'b'},
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
  default:
   for _,field:=range operationalFields {
    if fields[0]==field.name {
     if _,exists:=values[field.name];exists {return result}
     values[field.name]=fields[1]
     break
    }
   }
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
 for _,field:=range operationalFields {
  raw,ok:=values[field.name];if !ok {continue}
  n,err:=strconv.ParseUint(raw,10,64);if err!=nil {continue}
  value:=strconv.FormatUint(n,10)
  switch field.kind {
  case 'b':
   if n>1 {continue};value="Inactive";if n==1 {value="Active"}
  case 't':
   if n>253402300799 {continue};value="Never";if n!=0 {value=time.Unix(int64(n),0).UTC().Format(time.RFC3339)}
  }
  result.Operations=append(result.Operations,operationMetric{Label:field.label,Value:value})
 }
 result.Available=true
 return result
}
