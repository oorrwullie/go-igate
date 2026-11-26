<p align="center">
  <img src="assets/sparky-readme-banner.png" alt="Sparky, the Go gopher, operating a vintage field radio" width="100%">
</p>

[![Go CI](https://github.com/oorrwullie/sparky/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/oorrwullie/sparky/actions/workflows/go.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Sparky is an open-source radio data gateway written in Go. It receives, processes, and routes radio data between receivers, transmitters, and network services. Today it focuses on **APRS** and **ADS-B**, with room for additional protocols and hardware backends as the project evolves.

Sparky began as an RTL-SDR APRS iGate, inspired by [Ionosphere](https://github.com/cceremuga/ionosphere). Its current capabilities extend beyond an iGate.

**Simple. Powerful. Fast.**

New to radio? Visit [Welcome to the workbench](docs/learn/README.md). You can start with receive-only experiments and learn the radio fundamentals without installing Sparky or earning a license first.

## Why Sparky?

<img src="assets/sparky-gopher-mascot.png" alt="Sparky, the Go gopher wearing a radio headset" width="185" align="right">

The name is a nod to **Sparky**, the radio operator Radar frequently talks to in *M\*A\*S\*H*. He's the voice at the other end of the radio, helping messages get where they need to go. That's a fitting namesake for software that listens to radio traffic and routes it between the airwaves and connected services.

## Current capabilities

<p align="center">
  <img src="assets/sparky-radio-command-center.png" alt="Sparky operating a radio command center with APRS and ADS-B displays" width="850">
</p>

### APRS
- Receive APRS through an RTL-SDR with `rtl_fm` and `multimon-ng`, or from a sound-card audio input.
- Receive decoded APRS reports from OpenWebRX+ over MQTT (instead of the local receive decoder).
- Parse and deduplicate APRS packets, gate RF traffic to APRS-IS, and optionally gate eligible APRS-IS messages back to RF.
- Digipeat using configurable path and alias rules; transmit AFSK1200 through sound-card audio.
- Schedule RF and APRS-IS beacons, with optional aprs.fi verification and watchdog checks.

### ADS-B and MQTT
- Accept OpenWebRX+ MQTT reports using an external broker or an optional loopback-only embedded broker.
- Forward OpenWebRX+ ADS-B reports to map.flights over MQTT, with forwarding diagnostics.

**Scope:** ADS-B forwarding relies on OpenWebRX+ to decode aircraft reports; Sparky does not currently decode raw ADS-B samples itself. The embedded MQTT broker is provided for the local OpenWebRX+ integration, not as a general-purpose public broker. RF transmission currently uses a sound-card audio output and an external radio; native RF transmitter-module control is not yet implemented.

### Administration and observability
- Separate authenticated web configuration UI, with safe editing and optional fixed-target service restart.
- Optional process health, readiness and Prometheus metrics, plus a read-only runtime snapshot in the UI. These are not proof of RF reception or remote delivery.

## Getting started
Install prerequisites for your chosen backend, then build and configure Sparky. Start receive-only: review the example YAML and explicitly disable transmitter, digipeater and parent iGate before running a local listening experiment.

### Dependencies
**Build dependencies (all configurations):** `pkg-config` and the PortAudio development headers are required because the capture package currently imports PortAudio unconditionally, including for MQTT-only use. On Debian/Ubuntu, install `pkg-config` and `portaudio19-dev` before building.

**Runtime dependencies (depending on backend):**

* An RTL-SDR compatible device (for SDR reception).
* [rtl_fm](http://osmocom.org/projects/sdr/wiki/rtl-sdr) (for SDR reception)
* [multimon-ng](https://github.com/EliasOenal/multimon-ng) (for local audio decoding)
* [alsa-utils](https://alsa-project.org/wiki/Main_Page) (`aplay`) when using the sound-card transmitter

**Go toolchain:** Go 1.22.4 or later (see `go.mod`).

## Documentation and learning resources

- [Architecture and data flow](docs/architecture.md) — system, packet routing and administration diagrams.
- [Engineering decisions](docs/engineering-decisions.md) — design choices, trade-offs and implementation evidence.
- [Performance baseline](docs/performance-baseline.md) and [release acceptance checklist](docs/acceptance-testing.md) — reproducible measurement and validation.

- [Configuration reference](docs/configuration.md) · [Hardware compatibility](docs/hardware.md) · [Troubleshooting](docs/troubleshooting.md)
- [Upgrading](docs/upgrade.md) · [Known limitations](docs/limitations.md) · [Changelog](CHANGELOG.md)
- [Documentation index](docs/README.md) — installation, configuration, administration, and troubleshooting.
- [Welcome to the workbench](docs/learn/README.md) — friendly introductions to radio, licensing, and hands-on projects, including building your own gateway without Sparky. You don't need a callsign to start learning.

## Operator quick start

**New to Sparky?** Start with the [operator quick start](docs/operator-quickstart.md).
It covers building the gateway, launching the separate authenticated
configuration UI, saving and applying settings, enabling optional restart
controls, and displaying live gateway status. The UI is local-only by default
and no administrative restart permission is enabled automatically.

## Usage

### Receiving APRS from OpenWebRX+

OpenWebRX+ can publish decoded APRS reports to MQTT. By default, Sparky subscribes to an existing broker:

```yaml
openwebrx-mqtt:
  enabled: true
  broker: tcp://127.0.0.1:1883
  topic: openwebrx/APRS
  username: ""
  password: ""
```

With this enabled, MQTT replaces Sparky's RTL-SDR/sound-card receive decoder. Frames that contain a complete TNC2 packet are passed through the normal iGate and digipeater processing. The configured sound-card transmitter can still be used for RF transmission. Configure the broker and topic to match OpenWebRX+; its APRS report topic is usually `openwebrx/APRS`.

To remove Mosquitto from the APRS receive path, enable Sparky's embedded broker and point OpenWebRX+ at `127.0.0.1:1884` (SSL off; it is a same-machine connection):

```yaml
openwebrx-mqtt:
  enabled: true
  topic: openwebrx/APRS
  embedded-broker:
    enabled: true
    listen-address: 127.0.0.1:1884
map-flights:
  enabled: true
  broker: ssl://mqtt.map.flights:8883
  station: N0CALL
  key: "YOUR_MAP_FLIGHTS_STATION_KEY"
  topic: openwebrx/ADSB
```

The embedded broker binds only to loopback and allows anonymous connections, so OpenWebRX+ must run on the same machine. The map.flights station name is sent as the MQTT username and the station key as its password over TLS. Sparky forwards only reports whose topic matches `map-flights.topic` and whose JSON mode is `ADSB`; APRS and other OpenWebRX+ reports stay on the local path. Use a distinct map.flights key for this Sparky connection and disable the old Mosquitto bridge using that key, since map.flights allows one active connection per station key.

The feed sends aircraft reports as OpenWebRX+ produces them. The receiver must keep OpenWebRX+'s 1090 MHz ADS-B background decoder running continuously for a continuous feed. This removes the local Mosquitto hop from APRS processing and lets Sparky publish ADS-B directly; it does not change the time OpenWebRX+ takes to decode reports.

When map.flights forwarding is enabled, Sparky logs a once-per-minute ADS-B forwarding summary with incoming reports, completed QoS 0 MQTT publishes, failures, rejected reports, queue drops, queue depth, connection state, and age of the last received report. A completed QoS 0 publish only means Sparky's MQTT client completed the send; MQTT does not confirm that map.flights displayed the aircraft. Check the station page separately for ingestion and display.

### Build and run

```bash
make build       # Build both binaries
# Prepare bin/config.yml using the operator quick start before running.
(cd bin && ./sparky)
```

For service installation, follow the [operator quick start](docs/operator-quickstart.md) and review paths, permissions and config before running `make install-service`.

`make build` also produces `bin/sparky-config`, the separate authenticated
configuration interface. On Linux with systemd, opt into starting the UI
at boot with `make install-config-service` **after** setting up its
root-protected password environment file and non-root YAML access.
The UI runs independently so gateway restarts do not kill the admin connection.
See [configuration UI deployment](docs/web-config-deployment.md) for setup,
least-privilege requirements, and non-systemd deployment notes.

### Gateway systemd service and logs

For a **new** Linux systemd installation, the `make install-service` target
requires a non-root invoking account and will refuse to replace an existing
gateway unit. New installations write logs to the system journal:

```sh
sudo journalctl -u sparky.service -f
```

Read [gateway service security and migration](docs/gateway-systemd-security.md)
before installation, especially if you have an existing service, USB/audio
device permissions, or an older setup using `/var/log/aprs.log`.
The legacy `scripts/aprs.logrotate` file applies only to installations that
still use that file; it is **not** needed for new journald-based services.

### APRS-IS message gating

Message gating is disabled by default. To enable it, set `igate.message-gating: true` and keep the transmitter enabled. The gateway remembers stations heard directly over RF and gates APRS-IS messages only to those stations while they remain within `local-station-timeout` (one hour by default). It also avoids gating when the sender was recently heard on RF or the destination is already visible on APRS-IS, suppresses messages recently heard on RF and duplicate Internet deliveries for `message-dedup-window` (one minute by default), waits `message-gate-delay` (two seconds by default) to detect a local RF copy or another i-gate's transmission, includes a recent sender position when available, and spaces RF messages using `message-rf-interval` (five seconds by default). Messages are sent to RF using APRS third-party packet format and the configured `message-rf-path`.

When message gating is disabled, RF packets sent back to APRS-IS use `qAO`. When it is enabled, they use `qAR` to advertise that the gateway can gate messages to recently heard RF stations. A transmitter used only for beacons or digipeating does not by itself make the gateway bidirectional.

### Beacon configuration tips

- A home fill-in digipeater should advertise itself with the WIDE1-1 overlay. Use the alternate table digipeater symbol paired with the overlay character `S` (e.g. `=latNSlonW#...`) to match [APRS Protocol Reference v1.0.1, Chapter 2](http://www.aprs.org/doc/APRS101.PDF).
- Use a conservative beacon interval and path consistent with local APRS practice. Sparky currently supports the primary RF schedule; additional RF beacon entries are ignored. The current YAML parser defaults an empty primary beacon RF path to `WIDE1-1`; do not use `DIRECT` as a placeholder.
- `igate.beacon.disable-is-beacon: true` suppresses the station's own Internet beacons without disabling RF-to-IS gating. RF-only beacons are visible on aprs.fi only if an appropriate gateway receives and forwards them.
- Configuration changes are read at startup, so **restart the installed systemd unit** after editing `config.yml`; no rebuild is needed for configuration-only changes. Use `sudo systemctl restart sparky.service` for a new Sparky installation or `sudo systemctl restart aprs.service` for an existing installation that retains the original unit name.
- If aprs.fi occasionally drops your IGate, enable `igate.beacon.aprsfi-verification` with your aprs.fi API key and set `watchdog-interval` (for example `30m`). The watchdog polls aprs.fi and, when your call hasn't been heard for longer than `watchdog-max-age` (defaults to twice the interval), it automatically schedules an RF beacon to refresh the map.

## Renaming from go-igate

The application, Go module, and GitHub repository have been renamed to **Sparky**. The canonical module path is `github.com/oorrwullie/sparky` (previously `github.com/oorrwullie/go-igate`). Existing clones can update their remote with:

```bash
git remote set-url origin https://github.com/oorrwullie/sparky.git
```

Update any Go imports or automation that use the old module path, and review systemd service names and binary paths before deployment. The program binary and included service template are already named `sparky`. This rename does not introduce new radio behavior; keep your existing `config.yml` and verify the service configuration for your installation.

## Contributing

You don't have to be an expert in Go or radio to help build Sparky. Bug reports, questions, documentation corrections, hardware experiments and code contributions are all welcome.

- [Come help build Sparky!](CONTRIBUTING.md) — getting started, project principles and how to submit changes.
- [Open an issue](https://github.com/oorrwullie/sparky/issues/new/choose) — report a problem, suggest an idea, share a station report or ask a question.
- [Report security concerns privately](SECURITY.md).
- [Community conduct](CODE_OF_CONDUCT.md) · [Contributor acknowledgments](CONTRIBUTORS.md).

Even a small contribution can make Sparky better for the next person who comes along.

## License

Distributed under the MIT License. See `LICENSE` for more information.

## Repository
Project repository: [https://github.com/oorrwullie/sparky](https://github.com/oorrwullie/sparky)
