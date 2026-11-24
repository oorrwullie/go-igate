# Sparky operator quick start

This is the current guide to the gateway and its **separate**, authenticated
configuration UI. It documents behavior available on `main`; no RF hardware
or service installer is launched just by building the binaries.

## Before you start

- Go 1.22.4+ and the packages needed to build PortAudio (on Debian/Ubuntu:
  `pkg-config` and `portaudio19-dev`).
- For RTL-SDR receive, a supported dongle, `rtl_fm`, and `multimon-ng`.
  For OpenWebRX+ MQTT input, a reachable MQTT source instead of the local
  RTL-SDR receive decoder. Other backends have their own dependencies.
- A working Sparky `config.yml` for your chosen station and backend.
  The file is authoritative; the UI edits only a limited set of fields.
- Keep the gateway and administrative UI on the same host for the simplest
  loopback-only deployment.

## 1. Build and run the gateway

```sh
make build
cd bin
./sparky
```

Both `bin/sparky` and `bin/sparky-config` are produced by `make build`.
The gateway loads `config.yml` from its working directory; the command
above therefore expects `bin/config.yml`. Use the actual config location
from your existing installation. There is no automatic UI startup.

If using the bundled Linux systemd gateway unit, run
`make install-service` **only after reviewing its paths and permissions**.
It installs/starts a system service; it is not a dry run.

## 2. Start the separate configuration UI

Use an independently chosen 16+ character password. In a private terminal
session, set it without placing a literal secret in your shell history:

```sh
read -r -s SPARKY_CONFIG_PASSWORD
export SPARKY_CONFIG_PASSWORD
./bin/sparky-config --config ./bin/config.yml
```

Run these commands from the repository root. On the same machine open
`http://127.0.0.1:8080/` and authenticate with username `admin`.
For a remote browser, use an SSH tunnel, not an unauthenticated LAN port:

```sh
ssh -L 8080:127.0.0.1:8080 user@sparky-host
```

Replace `user@sparky-host` with your SSH destination. If local port 8080
is busy, choose another local port for the forward. Closing the UI process
does **not** stop the gateway. By default the UI binds only to loopback and
all pages require the password. See [web editor and network security](web-config-edit.md)
for HTTPS and explicitly restricted Routy reverse-proxy deployments.

## 3. Edit safely and apply changes

1. Review the saved configuration overview. It shows selected non-secret YAML
   values; it is **not** proof the running gateway loaded that exact revision.
2. Open **Edit settings** to change supported fields. Other settings, including
   secrets, remain in YAML and are not rendered into the browser.
3. Press **Save Changes**. If the YAML changed meanwhile, reload rather than
   overwriting it. A successful save writes to disk; it does **not** hot-reload.
4. Restart the gateway using your service manager. The web restart action is
   **disabled by default**, even when the UI is installed. The command
   `sudo systemctl restart sparky.service` is appropriate for a systemd
   installation with that unit name; verify yours before running it.
5. Verify service health and functional RF/network reception separately.
   A restart command's acceptance is not proof of a successful restart.

To opt into the web restart control, configure the UI at startup using the
appropriate supervisor, for example:

```sh
./bin/sparky-config --config ./bin/config.yml \
  --enable-restart --restart-manager systemd
```

The UI process requires *separately granted, narrowly targeted* restart
permission. Do not run the web UI as root or grant blanket sudo rights.
When enabled, the **Restart service** link on the overview opens the editor's
separate confirmation form; it never silently restarts on save.
See [restart permissions and supported supervisors](web-config-restart.md).

## 4. Optional live runtime status

The gateway can enable a loopback metrics listener via its environment:

```sh
(cd bin && SPARKY_OBSERVABILITY_ADDR=127.0.0.1:9091 ./sparky)
```

Launch the UI with an additional flag:

```sh
./bin/sparky-config --config ./bin/config.yml \
  --metrics-url http://127.0.0.1:9091/metrics
```

These examples assume the repo root as the working directory for the UI,
and the actual gateway config exists at `./bin/config.yml`.
If managed by systemd, set the environment in the gateway unit and add the
`--metrics-url` flag to the **UI unit**; simply running
`make install-config-service` does not enable either option.

The overview can then display gateway readiness, uptime, and admitted/dropped
**subscriber queue deliveries**. These counters are **not** unique received
RF packets, and the snapshot is fetched only when loading the overview.
No history or automatic polling is provided. Missing/unavailable metrics do
not prevent editing saved configuration.

See [observability](observability.md) and [runtime status](web-config-runtime-status.md)
for the exact security and networking boundaries.

## 5. Optional Linux systemd companion installation

The `make install-config-service` target installs the UI as a **separate**
systemd unit. Before using it, read
[the deployment guide](web-config-deployment.md): credentials must be in
`/etc/sparky/config-ui.env` owned by root with mode 0600; the installer must
be run by the **intended non-root UI account**; and that account must be able
to read/write the configuration plus its containing directory.

The bundled unit deliberately starts on loopback, with restart disabled
and without a metrics URL. For a remote operator, SSH port forwarding is
the simplest secure access. Production hardening should isolate writable
config from executable files; the bundled unit preserves the legacy shared
`bin/` working directory for compatibility.

## Troubleshooting cheatsheet

| Symptom | First check |
|---|---|
| UI returns 503 | `SPARKY_CONFIG_PASSWORD` must be set (16+ characters), and `--config` must point to valid, readable YAML |
| UI returns 401 | Use username `admin` and the configured independent password |
| Save rejected or returns 409 | Inspect validation message, then reload if the YAML was externally modified |
| Saved but no behavior change | Saving does not hot-reload; restart gateway using the configured supervisor |
| No Restart service link | Operator has not supplied `--enable-restart` and a valid fixed-target adapter |
| Restart request fails | Check the service manager, targeted privileges, and its logs; do not grant blanket root authority |
| Runtime status not configured/unavailable | Opt in to `SPARKY_OBSERVABILITY_ADDR` on the gateway and `--metrics-url` on the separate UI process; verify loopback and listener health |
| No decoded packets | Check selected receive backend, connected hardware, `rtl_fm`/`multimon-ng`, logs and configured frequency; web status is not an RF decoder diagnostic |

## Not yet included

Automatic discovery or guided hardware configuration, full-field GUI
editing, a historical dashboard, web-based logs, and a no-supervisor
"restart myself" mode are **not** provided. Configuration remains in YAML.
Do not confuse the GUI's saved snapshot with verified running settings.

For the most current code and issue status, check the repository; older
per-PR design notes in this docs directory may refer to earlier stages.
