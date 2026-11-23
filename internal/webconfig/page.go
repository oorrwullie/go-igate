package webconfig

const pageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sparky // Configuration</title>
<style>
:root{color-scheme:dark;--bg:#091117;--panel:#101c24;--line:#294450;--text:#cfe5e7;--muted:#88a4aa;--green:#8dff98;--cyan:#5bdfea}
*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 95% 0%,#143039 0%,transparent 33%),var(--bg);font:14px/1.5 ui-monospace,SFMono-Regular,Consolas,monospace;color:var(--text)}
header{border-bottom:1px solid var(--line);padding:18px 28px;display:flex;gap:24px;align-items:center;flex-wrap:wrap}
.brand{font-size:27px;font-weight:800;color:var(--green);letter-spacing:.04em}.sub{color:var(--muted);font-size:12px}.chip{border:1px solid #37624b;background:#12271e;padding:6px 12px;border-radius:5px;color:var(--green)}
.actions{margin-left:auto;display:flex;gap:8px}button{font:inherit;background:#182730;color:#738c93;border:1px solid #35515a;border-radius:5px;padding:9px 14px;cursor:not-allowed}
.layout{display:grid;grid-template-columns:208px minmax(0,1fr) 278px;gap:22px;max-width:1540px;margin:auto;padding:24px}
nav{border-right:1px solid var(--line);padding:12px 14px 0 0}nav a{display:block;padding:10px 12px;color:var(--muted);text-decoration:none;border-radius:5px}nav a.selected{color:var(--cyan);background:#102d35;border-left:3px solid var(--cyan)}
h1{font-size:25px;color:var(--green);margin:4px 0 0}h2{font-size:15px;color:var(--cyan);margin:0 0 12px}p{color:var(--muted);margin:5px 0 18px}
.card{padding:18px;border:1px solid var(--line);border-radius:7px;background:var(--panel);margin:0 0 16px;min-width:0}
.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.field{min-width:0}.field small{color:var(--muted);display:block;margin-bottom:5px}.value{border:1px solid #324f5b;padding:10px;background:#0b151c;border-radius:5px;overflow-wrap:anywhere;min-height:42px}
.state{color:var(--green)}.muted{color:var(--muted)}.note{font-size:12px;color:var(--muted)}.status{margin:12px 0}.status b{color:var(--green)}
@media(max-width:1050px){.layout{grid-template-columns:180px minmax(0,1fr)}aside{grid-column:2}}
@media(max-width:650px){.layout{display:block;padding:15px}nav{border-right:0;border-bottom:1px solid var(--line);display:flex;overflow:auto;gap:4px;margin-bottom:18px}nav a{white-space:nowrap}header{padding:14px}.actions{margin-left:0}.grid{grid-template-columns:1fr}}
</style></head><body>
<header><div><div class="brand">&gt;_ SPARKY</div><div class="sub">RADIO DATA GATEWAY // CONFIG</div></div><span class="chip">● VIEW ONLY</span>
<div class="actions"><a href="/edit" style="padding:9px 14px;border:1px solid var(--cyan);border-radius:5px">Edit settings</a><button disabled title="Saving is available on the Edit settings page">Save Changes</button><button disabled title="Service restart will be enabled in a later stage">Restart Service</button></div></header>
<div class="layout">
<nav aria-label="Sections"><a href="#overview" class="selected">~/ overview</a><a href="#aprs"># APRS / iGate</a><a href="#receive"># Receive</a><a href="#mqtt"># MQTT / ADS-B</a><a href="#system"># System</a></nav>
<main><h1 id="overview">$ sparky config --inspect</h1><p>Live configuration snapshot. Editing and restart controls are intentionally unavailable in this first slice.</p>
<section class="card" id="aprs"><h2>01 // Station &amp; APRS</h2><div class="grid">
<div class="field"><small>Callsign</small><div class="value">{{.Callsign}}</div></div>
<div class="field"><small>iGate</small><div class="value">{{if .IGate}}ENABLED{{else}}DISABLED{{end}}</div></div>
<div class="field"><small>APRS-IS</small><div class="value">{{if .APRSIS}}ENABLED{{else}}DISABLED{{end}}</div></div>
<div class="field"><small>Digipeater</small><div class="value">{{if .Digipeater}}ENABLED{{else}}DISABLED{{end}}</div></div>
</div></section>
<section class="card" id="receive"><h2>02 // RF &amp; Capture</h2><div class="grid">
<div class="field"><small>RTL-SDR</small><div class="value">{{if .SDR}}ENABLED{{else}}DISABLED{{end}}</div></div>
<div class="field"><small>SDR frequency</small><div class="value">{{.SDRFrequency}}</div></div>
<div class="field"><small>Audio transmitter</small><div class="value">{{if .Transmitter}}ENABLED{{else}}DISABLED{{end}}</div></div>
</div></section>
<section class="card" id="mqtt"><h2>03 // Network Inputs</h2><div class="grid">
<div class="field"><small>OpenWebRX MQTT</small><div class="value">{{if .MQTT}}ENABLED{{else}}DISABLED{{end}}</div></div>
<div class="field"><small>MQTT APRS topic</small><div class="value">{{.MQTTTopic}}</div></div>
<div class="field"><small>map.flights forwarding</small><div class="value">{{if .ADSB}}ENABLED{{else}}DISABLED{{end}}</div></div>
</div></section></main>
<aside id="system"><section class="card"><h2>// system status</h2><div class="status"><b>● READ-ONLY PREVIEW</b></div><p class="note">Values are read from config.yml on every request. No writes or service actions are exposed.</p><p class="note">Credentials, passcodes and API keys are intentionally omitted.</p></section><section class="card"><h2>// operations</h2><p class="note">Next: validated forms, safe save, and separately authorized service restart.</p></section></aside>
</div></body></html>`
