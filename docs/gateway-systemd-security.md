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
   The installer checks discoverable RTL-SDR USB and ALSA PCM permissions for
   the configured backend **before installing the unit**. Missing access
   stops installation and prints device-specific diagnostic commands. Do not
   put the gateway in the `root` group.
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

## Device access preflight and safe fixes

The installer uses `go run ./cmd/sparky-device-check --config bin/config.yml`
**as the intended non-root service user**, before any privileged unit writes.
The check follows Sparky's configured receive selection (OpenWebRX+ MQTT,
sound-card capture, or RTL-SDR), and checks sound-card output when TX is
enabled. It detects common RTL-SDR Realtek USB IDs and checks **only an unambiguously selected** device node for access (a unique default tuner or matching USB serial). When multiple tuners exist, librtlsdr's numeric indices cannot be reliably inferred from sysfs ordering, so the check prints an **advisory**, rather than rejecting unrelated devices or falsely certifying the selected one. Unknown serials and unsupported USB IDs are likewise advisory and require a manual `rtl_test -d` verification. It also checks accessible ALSA PCM nodes when needed. A confirmed device permission failure blocks installation with specific instructions. Run the same preflight manually after
changing group membership or connecting hardware:

```sh
go run ./cmd/sparky-device-check --config bin/config.yml
id -nG
lsusb
ls -l /dev/bus/usb/*/* /dev/snd/pcm* 2>/dev/null
```

Some RTL-SDR variants use different USB identifiers; if one is present but
the preflight does not recognize it, inspect `lsusb`, the device's udev rule,
and the `rtl_test` result before adjusting the device check. The checker
does not mutate udev rules, groups or ACLs. **Never** use unrestricted
`chmod 666`, blanket USB access, or run the gateway as root to get around
a failed check. If a detected node belongs to a narrowly scoped device
group (for example `plugdev`, `audio`, or `dialout`), an administrator
can grant access to that group *after reviewing the device scope*, then
fully log out and log back in. The script prints an example when applicable.

**Important limit:** readable/writable node permissions are a necessary
preflight, not a guarantee that an SDR driver will claim the interface or
that PortAudio selects the desired card. Desktop-session ACLs may be absent
under a headless systemd service even when the interactive login works.
Hardware and audio/PTT acceptance must be tested with the **actual service
account**, not only on the developer workstation. No serial/PTT device path
is available in the current YAML for a reliable automatic serial probe;
those require a specific deployment check.

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
