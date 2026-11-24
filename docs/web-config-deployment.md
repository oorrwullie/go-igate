# Run Sparky and its configuration UI together

The gateway (`sparky`) and configuration UI (`sparky-config`) are two
independent executables. **Do not embed the UI into the gateway process**:
the gateway must be able to restart without terminating the admin HTTP
connection or giving the gateway process new administrative privileges.

`make build` builds both binaries into `bin/`. Neither starts automatically
when built.

## Linux systemd deployment

The existing `make install-service` installs the gateway. The separate
`make install-config-service` command is an **explicit opt-in** for the
administrative UI. It installs and enables an additional `sparky-config.service`
unit which survives restarts of `sparky.service`.

1. Place `config.yml` in the same `bin/` working directory already used by
   `sparky.service`. Ensure the non-root account running the UI can read and
   write the file **and its containing directory**. Do not make `config.yml`
   world-writable. The existing config writer atomically replaces the file,
   so directory write access is needed.
2. As an administrator, create `/etc/sparky/config-ui.env` with mode 0600
   and an independent, randomly chosen password (16+ characters):

   ```sh
   sudo install -d -m 0755 /etc/sparky
   sudo sh -c 'umask 077; printf "SPARKY_CONFIG_PASSWORD=%s\n" "$1" > /etc/sparky/config-ui.env' sh 'REPLACE-WITH-A-LONG-RANDOM-PASSWORD'
   sudo chmod 0600 /etc/sparky/config-ui.env
   ```

   For a real deployment avoid leaving the password in shell history:
   create the file using a secure editor or secrets-management system instead.
   The example is intentionally illustrative, not a recommended way to enter
   real secrets.
3. Run these commands **from the repository as the intended non-root UI account**:

   ```sh
   make build
   make install-config-service
   ```

   The install target checks for credentials, installs the companion unit,
   reloads systemd, and enables it at boot. It does **not** modify or restart
   the main Sparky unit.

4. On the same host, visit `http://127.0.0.1:8080/` and authenticate as
   `admin` with the configured password. For a remote browser, use SSH port
   forwarding, not a public plaintext port:

   ```sh
   ssh -L 8080:127.0.0.1:8080 user@sparky-host
   ```

The companion service binds only to loopback, uses its own non-root account,
and reads its password from a root-protected environment file. The HTTP
interface's existing authentication, CSRF, Origin and Host protections remain
active.

The companion is **not** installed as a requirement or child of
`sparky.service`. A gateway restart does not stop the UI, and restarting the
UI does not reset the radio gateway.

To check or disable the UI:

```sh
sudo systemctl status sparky-config.service
sudo journalctl -u sparky-config.service
sudo systemctl disable --now sparky-config.service
```

### Security limits

The initial template retains the existing gateway's `bin/` working directory
to avoid silently moving configuration. Because the config store replaces
YAML atomically, the UI user requires write access to that directory. For
a hardened installation, place configuration in a **dedicated writable
directory outside the executable directory**, then update the gateway unit
and the UI unit's `--config` path and `ReadWritePaths` accordingly. The
bundled template does not provide the strongest possible isolation when
executables and configuration share a writable directory.

Restart authority remains **disabled** in this companion unit. PR #49 adds a
separately authorized `--enable-restart` operation. After it lands, configure
its manager and least-privilege permissions separately; do not enable it by
default or make this service run as root.

## Other environments

- **macOS:** build both binaries and configure two distinct launchd jobs:
  one for the gateway, one for `sparky-config`, with the UI running as a
  non-privileged account. A packaged launchd installer is not included in
  this Linux-systemd slice.
- **Kubernetes:** put gateway and UI in independently managed containers or
  processes, with a shared persistent configuration volume and independent
  credentials. The restart adapter in #49 targets a Deployment using
  narrowly scoped Kubernetes RBAC. Do not make a UI container depend on
  the gateway container remaining alive during its rollout.
- **Other Linux init systems:** supervise both binaries independently. The
  restart adapter selected in #49 is not a replacement for an installation
  or process supervisor.

This PR changes packaging/deployment only. It does not change YAML storage,
authentication, radio behavior, the admin listener, or restart permissions.
