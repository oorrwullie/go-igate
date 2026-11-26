# Configuration reference

The gateway reads config.yml from its working directory at startup. The separate administration UI takes a configuration path and edits a subset of these fields. Restart the gateway explicitly after saving. Keep credentials and station-specific details private.

This reference describes the current implementation; the [example YAML](../config.yml.example) is not a list of implicit runtime defaults. Unspecified booleans are false, strings empty, and numeric fields zero unless the parser or backend supplies a fallback. Durations use Go duration strings (e.g. 30s, 5m, 1h). Quote values passed to external programs as strings.

## Receive-only starting point

For North American terrestrial APRS, a local RTL-SDR listening configuration can start with the following. Adjust gain, frequency and hardware selector for your actual receiver and region. No callsign or network credentials are needed for this local-only experiment.

```yaml
station-callsign: ""
cache-size: 1000
enable-digipeater: false
soundcard-capture-enabled: false
sdr:
  enabled: true
  frequency: "144.390M"
  device: "0"
  gain: "20"
  ppm-error: "0"
  squelch-level: "0"
  sample-rate: "22050"
multimon:
  path: multimon-ng
transmitter:
  enabled: false
igate:
  enabled: false
  beacon:
    enabled: false
openwebrx-mqtt:
  enabled: false
map-flights:
  enabled: false
```

Keep your transmitter disconnected for the initial receive-only test. Do not overwrite an existing station configuration. See the [quick start](operator-quickstart.md).

## Top-level settings

| Key | Runtime default / behavior | Purpose |
|---|---|---|
| station-callsign | Empty; required by APRS-IS and beacon operation | Your authorized station callsign and optional SSID |
| cache-size | Values <= 0 become 1; example 1000 | Maximum input deduplication entries; gateway uses 60s TTL and best-effort .cache.json persistence |
| enable-digipeater | false | Enable digipeating; transmitter required |
| soundcard-capture-enabled | false | Explicitly select sound-card reception over SDR |
| soundcard-input-name | Empty, default input | PortAudio device selection |
| soundcard-output-name | Empty, default output | PortAudio output or ALSA device when using aplay |

OpenWebRX+ MQTT takes precedence over local capture. Otherwise an explicitly enabled sound card wins over SDR; if neither local selector is enabled, the implementation falls back to sound-card capture. Turning off SDR alone is not a way to disable reception.

## sdr

All CLI argument values below are strings.

| Key | Example / behavior | Purpose |
|---|---|---|
| enabled | false if omitted | Select RTL-SDR when not using MQTT or explicit sound-card capture |
| frequency | "144.390M" | Tune frequency; check local band plan |
| sample-rate | "22050" | Decoder audio sample rate |
| device | Empty or "0" uses rtl_fm default | Index or supported serial selector |
| gain | "20" | Tuner gain; adjust for noise and overload |
| ppm-error | "0" | Frequency correction |
| squelch-level | "0" | rtl_fm squelch setting |
| additional-flags | Empty | Extra whitespace-separated rtl_fm arguments; avoid conflicts |
| path | Parsed but not used by current capture | Executable is currently fixed to rtl_fm on PATH |

The historical example key sdr.exec is not recognized by the config struct and does not change the executable. These CLI arguments are not all validated by the YAML parser.

## multimon

| Key | Default | Purpose |
|---|---|---|
| path | multimon-ng | Decoder executable |
| additional-flags | Empty | Whitespace-separated extra decoder arguments |
| receive-diagnostics | false | Enable periodic decoder-input and sound-card diagnostics |

The local decoder expects 22050 Hz raw mono audio for AFSK1200.

## openwebrx-mqtt

| Key | Default / requirement | Purpose |
|---|---|---|
| enabled | false | Replace local capture with decoded MQTT reports |
| broker | Required in external mode | Broker URI, e.g. tcp://127.0.0.1:1883 |
| topic | openwebrx/APRS | APRS subscription filter |
| username | Empty | Optional MQTT authentication |
| password | Empty | Secret MQTT password |
| embedded-broker.enabled | false | Use local embedded broker instead |
| embedded-broker.listen-address | 127.0.0.1:1884 | Loopback only (literal loopback IP or localhost) |

The embedded broker permits anonymous local clients and has no TLS. Restrict access to trusted local producers; reports are treated as received RF traffic.

## map-flights

| Key | Default / requirement | Purpose |
|---|---|---|
| enabled | false | Forward decoded ADS-B reports; requires MQTT input |
| broker | ssl://mqtt.map.flights:8883 | TLS MQTT endpoint; ssl:// required |
| station | Required when enabled | Station identifier / MQTT username |
| key | Required when enabled | Secret station key / MQTT password |
| topic | openwebrx/ADSB | Concrete input and publish topic; no wildcards |

Sparky forwards reports, not raw samples. MQTT completion is not remote map confirmation.

## transmitter

| Key | Default | Purpose |
|---|---|---|
| enabled | false | Enable sound-card TX |
| tx-delay | 1s for nonpositive input | AFSK preamble duration |
| tx-tail | 100ms for nonpositive input; example 75ms | Tail tone duration |
| audio-level | 0.9 when zero/omitted | Normalized amplitude, range 0–1; zero selects fallback, not mute |
| use-aplay | false | ALSA aplay playback instead of PortAudio |

Legacy transmitter.baud-rate and transmitter.read-timeout are not parsed; AFSK1200 is fixed. The audio path needs a suitable radio and PTT/VOX arrangement. Do not enable TX until appropriately licensed and hardware has been verified.

## digipeater

| Key | Default | Purpose |
|---|---|---|
| alias-patterns | ^WIDE1-1$ | Regular expressions matching fill-in aliases |
| wide-patterns | WIDE, TRACE, HOP and two-letter regional patterns with 1–7 hop fields | Regular expressions; see example for exact expressions |
| ssn-prefixes | Empty list | Optional regional prefixes, normalized uppercase |
| dedupe-window | 30s when zero | Suppress repeated digipeater traffic |

Choose paths and aliases consistent with local policy. An empty pattern list selects defaults rather than disabling the rules.

## igate

| Key | Default / requirement | Purpose |
|---|---|---|
| enabled | false | Start iGate; current constructor opens APRS-IS when enabled |
| message-gating | false | Allow eligible Internet-to-RF messages; requires TX |
| message-rf-path | Empty (direct) | AX.25 path, up to eight components; no Internet-only markers or used-hop stars |
| message-rf-interval | 5s | Minimum RF message spacing |
| message-dedup-window | 1m | Internet message duplicate window |
| message-gate-delay | 2s | Wait for local RF copy before gating |
| local-station-timeout | 1h | Recently heard station eligibility |
| forward-self-rf | false | Permit eligible self-originated RF forwarding |
| gate-digipeated-path | false | Apply digipeater rewrite to gated paths when digipeater is enabled |

Nonpositive message durations select their defaults. The iGate needs valid APRS-IS connection settings even when its nested enabled flag is false.

### igate.aprsis

| Key | Default / requirement | Purpose |
|---|---|---|
| enabled | false; not an effective connection gate in current constructor | Use parent igate.enabled to prevent connection |
| server | Required for iGate | Host:port, e.g. noam.aprs2.net:14580 |
| passcode | Required for iGate | Authorized APRS-IS passcode; secret |
| filter | Empty | Server filter; range filter also informs local geographic forwarding policy |
| mute-inbound-log | false | Suppress inbound packet logging |

### igate.beacon

| Key | Default / behavior | Purpose |
|---|---|---|
| enabled | false | Enable beacon scheduling |
| interval | Zero | Legacy fallback for RF and IS intervals |
| rf-interval | Falls back to interval | Primary RF interval; positive configured schedule must be at least 10m |
| is-interval | Falls back to interval | Primary Internet interval; same minimum |
| disable-rf | false | Suppress RF beacons |
| disable-tcp | false | Legacy flag suppressing own Internet beacons, not the APRS-IS connection |
| disable-is-beacon | false | Suppress own Internet beacons while retaining forwarding |
| max-rf-attempts | 3 | Maximum RF beacon attempts |
| comment | Empty | APRS information field; supply valid position/status data as appropriate |
| rf-path | WIDE1-1 if empty | Primary RF path (empty YAML falls back to this default) |
| is-path | TCPIP* if empty | Internet beacon path |
| additional-rf-beacons | Empty | Parsed but currently ignored; secondary schedules are not implemented |

Use conservative beacon settings. Do not copy example coordinates or callsigns onto the air.

### igate.beacon.aprsfi-verification

| Key | Default | Purpose |
|---|---|---|
| enabled | false | Optional aprs.fi verification |
| api-key | Empty | Required to activate verification; secret |
| delay | 10s | Delay before verification |
| max-attempts | 3 | Verification attempts |
| timeout | 10s | HTTP client timeout |
| watchdog-interval | 0 (disabled) | Optional periodic visibility check |
| watchdog-max-age | Twice watchdog interval when unset and watchdog enabled | Age threshold for watchdog |

Watchdog behavior can schedule RF beacons; enable only with a validated, authorized station.

## Process settings (not YAML)

| Setting | Default / behavior |
|---|---|
| SPARKY_OBSERVABILITY_ADDR | Unset disables listener; e.g. 127.0.0.1:9091 |
| SPARKY_OBSERVABILITY_ALLOW_REMOTE | Set to 1 only for explicitly restricted remote scraping; endpoints unauthenticated |
| SPARKY_CONFIG_PASSWORD | Required for UI access; at least 16 characters |
| sparky-config --listen | 127.0.0.1:8080 |
| --config | config.yml relative to UI working directory |
| --tls-cert / --tls-key | PEM paths for HTTPS |
| --public-origin / --trusted-proxy-ip / --allow-lan-http | Explicit restricted reverse-proxy options; see deployment guide |
| --metrics-url | Unset; literal loopback HTTP /metrics URL |
| --enable-restart | false; requires explicit OS permission |
| --restart-manager | auto; see supported supervisors |
| --restart-target | Manager-specific default when empty |
| --restart-helper | Empty; absolute fixed helper for custom adapter |

See [deployment](web-config-deployment.md), [restart controls](web-config-restart.md), and [observability](observability.md) for security boundaries. Keep the UI loopback-only unless you understand and have configured the supported secure remote access options.
