# Gateway systemd installation: least-privilege baseline

Sparky's gateway (`sparky`) and configuration UI (`sparky-config`) are
separate systemd services. This document concerns **the gateway only**.

## New installations

The bundled `make install-service` target is for a **new** service. It
deliberately refuses to overwrite any existing `sparky.service` in
systemd's unit search path. It also refuses to run when `make` itself
is executed as root or the selected gateway user differs from the caller.
Only fixed unit installation and service management use `sudo`.

1. Create a dedicated non-root account or choose an existing limited account.
   Run build and installation *as that account*, not with `sudo make`.
   Confirm the account has permission to access its chosen SDR/serial/audio
   devices; do not put the gateway in the `root` group.
2. Configure `bin/config.yml` with your station identity and receive backend.
   `make init-config` (once available from PR #56) copies a non-overwriting
   receive-only sample. For older revisions, create/review the file manually.
   The installer refuses missing or symlinked configuration.
3. Make sure the gateway account can **write** `bin/`. The current Go
   gateway saves its deduplication cache as `.cache.json` relative to the
   working directory. A dedicated writable state directory is preferable but
   requires a separate application change.
4. From the repository root, as that non-root account:

   ```sh
   make build
   make install-service
   sudo systemctl status sparky.service
   sudo journalctl -u sparky.service -f
   ```

The newly installed unit has `User=<invoking user>`, `UMask=0077`,
`NoNewPrivileges=true`, `PrivateTmp=true` and
`ProtectSystem=strict` with one `ReadWritePaths` exception for the
existing working directory. The unit does not need its user in `www-data`.
Logs are sent to **journald**, not `/var/log/aprs.log`.
Journal retention follows host journal settings.

**Compatibility/security tradeoff:** The working directory also contains
executables and the configuration file; write access to it is broader than
ideal. We retain that model for compatibility with existing cache/config
semantics. A future dedicated config/state layout can reduce the writable
surface after separate migration and hardware tests.

## Existing installations: no silent migration

An older service may run as root, use `Group=www-data`, and append to
`/var/log/aprs.log`. The installer refuses to replace it. Inspect and
migrate intentionally after checking the effective unit and ownership:

```sh
sudo systemctl cat sparky.service
sudo systemctl show sparky.service -p User -p Group -p WorkingDirectory -p ExecStart
sudo ls -l /var/log/aprs.log
```

Consider whether the device permissions for `rtl_fm`, PortAudio, sound
cards, serial PTT, and any hardware modules work for the new account.
Ensure the selected account can read `config.yml` and update the cache.
Back up the existing unit and configuration before updating them. If the
configuration UI runs as another account, it also requires appropriate
write access to its config directory; never solve this with world-writable
permissions.

The bundled installer does **not** edit an existing unit, change account
ownership, stop or restart an installed gateway, delete the old log file,
modify journald policy, or add device group membership.

For the old log file and `scripts/aprs.logrotate`: keep any historical
rotation policy only if the old deployment still uses it. New installations
using journald should not install that legacy policy.

## Verification checklist

- Verify `systemctl show sparky.service -p User` lists a **non-root** account.
- Check `journalctl -u sparky.service` for successful startup and errors.
- Verify radio receiver, MQTT/APRS-IS reachability and optional TX/PTT
  separately; a successful service activation does not prove RF operation.
- Stop and start service once, checking shutdown and retained cache.
- Test actual hardware access on the host; GitHub CI cannot validate local
  USB/audio device permissions.
- Keep the admin UI separate, loopback-only by default, independently
  authenticated and without default restart authority.

This change does not modify the runtime packet path, change an operator's
existing systemd unit, or grant additional privileged commands to the UI.
