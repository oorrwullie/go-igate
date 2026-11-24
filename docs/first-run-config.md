# First-run station configuration (safe defaults)

A new build produces two binaries, but it does **not** create station
credentials or automatically start a receiver, digipeater, or transmitter.
Use this explicit initialization step from the repository root:

```sh
make build
make init-config
```

This creates `bin/config.yml` by copying `config.yml.example`, with mode
`0600`. The command **refuses to overwrite** an existing file or an existing
symlink. If you already have an operational `config.yml`, do not run it.

The example intentionally has **transmitter disabled** and **digipeater
disabled**. Both can cause RF transmissions and must be enabled separately
after checking your sound-card/radio/PTT wiring and the rules for your license
and station. This change affects only newly created configurations, not
installed operator files.

## Before your first start

1. Edit `bin/config.yml` and supply your actual
   `station-callsign`; the example leaves it blank on purpose.
2. Confirm the receive backend. The example enables RTL-SDR on
   `144.390M` (the North American APRS frequency), which is not necessarily
   appropriate outside that region. The receiver requires `rtl_fm`,
   `multimon-ng`, and a compatible device. If you use OpenWebRX+ MQTT
   instead, configure its topic/broker and select that receive backend.
3. Decide whether to enable APRS-IS. The example leaves APRS-IS networking
   disabled, and the receive-only station will not gate packets to APRS-IS
   until you configure it.
4. Avoid enabling RF beaconing, sound-card TX, or digipeating until
   the transmit chain has been tested and approved for the station.
5. Run `cd bin && ./sparky` from the repository root. Sparky loads
   `config.yml` relative to its working directory. Run the separate
   web UI from the repo root with
   `./bin/sparky-config --config ./bin/config.yml` after supplying
   its independent admin password (see [web editor setup](web-config-edit.md)).

## Existing installations

This setup command does not modify existing configurations. Neither
`make build` nor `make init-config` installs a service or activates TX.
Do not use `make install-service` until you've inspected the bundled
systemd unit: its current gateway template specifies `Group=www-data`
but **does not specify a non-root `User=`**. The gateway service privilege
model and log paths need a separate security review before a general
production installation recommendation.

The bundled companion admin service is separate, opt-in, and has its own
credential/permission requirements. See
[web configuration deployment](web-config-deployment.md).

## Verify before enabling RF output

If you intend to digipeat, beacon, or gate to RF, configure an appropriate
radio, PTT and audio levels; verify what service is allowed for your
license and location; then deliberately set `transmitter.enabled` and/or
`enable-digipeater` to true. The sample's disabled settings protect
first-time users from accidental transmission, but do not replace a
functional TX/PTT interlock or RF testing.
