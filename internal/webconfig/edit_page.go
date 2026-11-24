package webconfig

const editHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Sparky // Edit Configuration</title><style>
:root{color-scheme:dark}*{box-sizing:border-box}body{margin:0;background:#081219;color:#d7e7ea;font:14px/1.65 ui-monospace,SFMono-Regular,Consolas,monospace}header{padding:20px;border-bottom:1px solid #304e57;display:flex;gap:20px;align-items:center;flex-wrap:wrap}h1{font-size:25px;color:#93ffa3;margin:0}h2{color:#55dce9;font-size:16px}main{max-width:810px;margin:30px auto;padding:0 18px}.card{background:#102028;border:1px solid #36545d;border-radius:7px;padding:20px;margin-bottom:18px}label{display:block;margin:13px 0}input[type=text]{background:#09171d;color:#e4f7f2;padding:10px;border:1px solid #426770;border-radius:4px;font:inherit;width:100%}input[type=checkbox]{accent-color:#93ffa3;width:17px;height:17px;vertical-align:middle}a{color:#60dce7}button{padding:10px 20px;border:1px solid #64e5e9;color:white;background:#176f77;border-radius:5px;font:inherit;cursor:pointer}.info{color:#a9c0c5}.success{padding:10px;border-left:3px solid #93ffa3;background:#163525}
</style></head><body><header><h1>&gt;_ SPARKY / CONFIG EDITOR</h1><a href="/">Overview</a></header><main>
{{if .Saved}}<p class="success">Configuration saved. Restart is required to apply changes.</p>{{end}}
<p class="info">Only selected settings can be edited. Advanced fields, YAML comments, and credentials are retained. Saving never restarts the service.</p>
<form method="post" action="/edit"><input type="hidden" name="csrf" value="{{.Token}}"><input type="hidden" name="revision" value="{{.Revision}}">
<section class="card"><h2>01 // Station identity</h2><label for="callsign">Callsign</label><input id="callsign" name="callsign" type="text" maxlength="32" value="{{.Values.Callsign}}"></section>
<section class="card"><h2>02 // Services</h2>
<label><input type="checkbox" name="sdr" {{if .Values.SDR}}checked{{end}}> RTL-SDR receiver</label>
<label><input type="checkbox" name="igate" {{if .Values.IGate}}checked{{end}}> APRS iGate</label>
<label><input type="checkbox" name="digipeater" {{if .Values.Digipeater}}checked{{end}}> Digipeater</label>
<label><input type="checkbox" name="transmitter" {{if .Values.Transmitter}}checked{{end}}> Sound-card transmitter</label>
<label><input type="checkbox" name="mqtt" {{if .Values.MQTT}}checked{{end}}> OpenWebRX+ MQTT input</label>
<label><input type="checkbox" name="adsb" {{if .Values.ADSB}}checked{{end}}> map.flights ADS-B forwarding</label>
</section><button type="submit">Save Changes</button><p class="info">Restart Service will be enabled in a separately reviewed change.</p></form></main></body></html>`
