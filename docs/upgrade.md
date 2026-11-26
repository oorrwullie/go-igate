# Upgrading and migrating from go-igate

Sparky was originally go-igate. The canonical repository and Go module are now github.com/oorrwullie/sparky. A rename does not automatically migrate your station.

## Before changing a working station

Record the running version/commit, actual service name, user, working directory, config path and binary path. Review the unit with systemctl cat and check the effective user, ExecStart and WorkingDirectory. Keep a private backup of the configuration, cache, service definition, existing binaries and any drop-ins. Do not put credentials in Git.

For an existing Git clone, update its remote if appropriate:

```sh
git remote set-url origin https://github.com/oorrwullie/sparky.git
git fetch origin
```

If your origin is your own fork, keep it and update the upstream remote instead.

## Build and validate the candidate

Build the chosen reviewed revision in a separate directory with the documented dependencies. Compare its example YAML and [configuration reference](configuration.md) against your existing configuration. Review changed defaults and service permissions; do not blindly replace your working file with the sample.

Use a separate receive-only test configuration. Avoid running two receivers against one dongle, two TX controllers on one radio, or two publishers with the same service credentials. Test changes in an offline fixture when possible.

## Switch deliberately

Schedule downtime, stop the existing gateway using its actual supervisor, and preserve the backed-up binary/config. Install the candidate using the intended account and reviewed paths. The bundled service name is sparky.service, but older installations may use aprs.service; do not enable both. Do not run a first-install target against an existing unit without reviewing its behavior. The legacy template may run with elevated privileges, so inspect the unit before use.

Start the selected service and confirm local decoding, credentials, network delivery, logs and clean shutdown. Re-enable transmission only after verifying your authorized station's RF chain. The separate admin UI is opt-in; see [deployment](web-config-deployment.md).

## Rollback

If validation fails, stop the new process, restore the previous binaries and configuration and any changed unit/drop-ins, reload the service manager if applicable, and start the previously verified service. Check actual RF and network behavior again. Preserve failure logs privately for investigation.

No automatic migration, binary compatibility or lossless cache migration is promised. See the [release acceptance checklist](acceptance-testing.md).
