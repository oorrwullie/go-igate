package aprs

import "testing"

// Fixed representative inputs make results comparable across revisions.
// These measure decoded-text processing, NOT SDR acquisition or AFSK decoding.
var benchmarkFrames = []struct {
 name string
 frame string
}{
 {"position", "N0CALL-9>APRS,WIDE1-1:!4010.20N/11137.80W-Test"},
 {"message", "N0CALL-9>APRS,WIDE1-1::TARGET   :hello world{42"},
 {"thirdparty", "N0CALL-9>APRS,WIDE2-1:}W7ABC>APRS,TCPIP*:>status"},
}

var benchmarkPacketSink *Packet
var benchmarkTypeSink PacketType
var benchmarkValidSink bool

func BenchmarkParsePacket(b *testing.B) {
 for _,tc:=range benchmarkFrames {
  b.Run(tc.name,func(b *testing.B){
   b.ReportAllocs()
   var p *Packet
   for i:=0;i<b.N;i++{
    var err error
    p,err=ParsePacket(tc.frame)
    if err!=nil{b.Fatal(err)}
   }
   benchmarkPacketSink=p
  })
 }
}

func BenchmarkParseAndClassify(b *testing.B) {
 for _,tc:=range benchmarkFrames {
  b.Run(tc.name,func(b *testing.B){
   b.ReportAllocs()
   var p *Packet
   var packetType PacketType
   for i:=0;i<b.N;i++{
    var err error
    p,err=ParsePacket(tc.frame)
    if err!=nil{b.Fatal(err)}
    packetType=p.Type()
   }
   benchmarkPacketSink=p
   benchmarkTypeSink=packetType
  })
 }
}

func BenchmarkPositionDecode(b *testing.B) {
 cases:=[]struct{name,frame string}{
  {"uncompressed","N0CALL>APRS:!4010.20N/11137.80W-Test"},
  {"compressed","N0CALL>APRS:!/:21D2:OF>  !Test"},
 }
 for _,tc:=range cases {
  b.Run(tc.name,func(b *testing.B){
   p,err:=ParsePacket(tc.frame)
   if err!=nil{b.Fatal(err)}
   b.ReportAllocs()
   var valid bool
   for i:=0;i<b.N;i++{
    _,_,valid=p.Position()
    if !valid{b.Fatal("fixture is not a valid APRS position")}
   }
   benchmarkValidSink=valid
  })
 }
}
