package pubsub

import "testing"

// Unlike a receive-only benchmark, the fast-path consumer is drained every
// iteration so the bounded queue does not silently transition to drop mode.
func BenchmarkFanoutAdmitted(b *testing.B) {
 for _,count:=range []struct{name string; subscribers int}{
  {"one",1},{"two",2},{"four",4},
 } {
  b.Run(count.name,func(b *testing.B){
   p:=New()
   channels:=make([]<-chan string,count.subscribers)
   for i:=range channels{channels[i]=p.Subscribe()}
   const frame="N0CALL>APRS,WIDE1-1:!4010.20N/11137.80W-Test"
   b.ReportAllocs()
   b.ResetTimer()
   for i:=0;i<b.N;i++{
    p.Publish(frame)
    for _,ch:=range channels{
     if _,ok:=<-ch;!ok{b.Fatal("subscriber closed unexpectedly")}
    }
   }
   b.StopTimer()
   if got:=p.Drops();got!=0{b.Fatalf("unexpected dropped deliveries: %d",got)}
   if got:=p.Published();got!=uint64(b.N*count.subscribers){b.Fatalf("deliveries=%d want %d",got,b.N*count.subscribers)}
  })
 }
}

// A congested subscriber must never make the producer wait; this measures
// the bounded queue's saturated-drop path separately from admitted fanout.
func BenchmarkFanoutSaturated(b *testing.B){
 p:=New()
 _=p.Subscribe()
 const frame="N0CALL>APRS:>test"
 for i:=0;i<subscriberQueueSize;i++{p.Publish(frame)}
 b.ReportAllocs()
 b.ResetTimer()
 for i:=0;i<b.N;i++{p.Publish(frame)}
 b.StopTimer()
 if got:=p.Drops();got!=uint64(b.N){b.Fatalf("drops=%d want %d",got,b.N)}
}
