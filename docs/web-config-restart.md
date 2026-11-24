# Opt-in service restart (standalone configuration UI)

Sparky's configuration editor saves YAML but does not apply changes to an
already-running gateway. This change adds a deliberately optional **Restart
Sparky** action. It is not enabled by default.

The configuration UI remains a **separate process** (`sparky-config`). This
separation is important: the main gateway can restart without killing the
requesting web UI.

## Enable

On a Linux system with systemd and an installed service named
`sparky.service`, explicitly pass `--enable-restart`:

```sh
export SPARKY_CONFIG_PASSWORD='a-long-unique-random-password'
go run ./cmd/sparky-config --config ./config.yml --enable-restart
```

The admin browser must log in, open **Edit settings**, tick the restart
confirmation checkbox, then press **Restart Sparky**. Save Changes and Restart
Sparky are independent operations. The restart request never saves the form.

When enabled, the adapter executes exactly
`/usr/bin/systemctl restart sparky.service` with no shell and no
browser-supplied arguments, service names, executable paths, or configuration
paths. The operation has a 15-second context timeout and concurrent requests
are refused while one is running. Successful return means systemctl accepted
the operation, **not** that the gateway restarted successfully and is healthy.

## Permissions and hardening

The web UI **does not acquire root permissions, run sudo, install policy
rules, or alter the service unit**. The account running `sparky-config` must
already have permission to restart the *specific* `sparky.service` unit.
Without that separate authorization the request fails closed and tells the
operator to inspect the service-manager logs.

Use a dedicated, unprivileged UI service account and configure a narrowly
scoped systemd/polkit authorization for this specific unit and operation if
the deployment requires browser-initiated restart. Do **not** grant generic
`systemctl`, arbitrary-unit management, shell access, or passwordless
unrestricted sudo to the UI user. Avoid running `sparky-config` as root.

Restart is a privileged action. Protect the UI with the existing Basic auth
and TLS/loopback policies. If Routy terminates TLS on another machine, follow
the documented trusted-proxy and firewall restrictions; the backend HTTP hop
is otherwise plaintext. The server checks authentication, the actual TCP
source/Host, the exact expected Origin, CSRF token, and explicit form
confirmation. Cross-site and GET requests cannot trigger the operation.

The endpoint is POST `/restart` and returns 404 when disabled. It sends no
adapter error details to the browser. Repeated authenticated requests after
an operation finishes can request another restart; this is an administrative
control, **not** rate-limited or a health monitor.

## Limitations

- This command targets only a Linux systemd unit named `sparky.service`.
  Different unit names, other init systems, and container orchestration are
  deliberately not supported by this initial adapter.
- The systemd binary must exist at `/usr/bin/systemctl`.
- The web UI must remain a separate process until an explicitly reviewed
  in-process lifecycle mechanism exists. Do not run the restart adapter from
  the Sparky service process itself.
- This phase does not add a live health probe, automatic rollback, or an
  authoritative restart-pending indicator. The existing `/edit?saved=1`
  banner only means the save URL was requested with that query parameter.
