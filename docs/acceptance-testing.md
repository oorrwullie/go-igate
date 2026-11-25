# Release acceptance checklist

Use this checklist on a clean installation of the candidate commit or tag. Record the exact version, OS, architecture, hardware and results. Mark skipped scenarios explicitly; a green CI run is not real-radio acceptance.

## Clean installation and receive-only

- [ ] Install documented dependencies as a normal user; build both binaries.
- [ ] Create a separate configuration without overwriting an existing file. Explicitly disable TX, digipeating, beacons and iGate forwarding before local reception.
- [ ] Confirm actual service-account USB or sound-card access; do not use blanket device permissions or run the admin UI as root.
- [ ] Receive and decode a known signal or recording; document the input source and observed results.
- [ ] Restart the gateway and confirm cache persistence and expected configuration.

## Operator UI and security

- [ ] Unauthenticated access fails; authenticated overview works.
- [ ] Invalid edits fail; concurrent external edits do not get overwritten.
- [ ] Save valid settings, verify on disk, confirm no implicit restart.
- [ ] Restart control is disabled by default; explicitly enabled fixed-target restart requires confirmation and narrowly scoped privileges.
- [ ] UI remains accessible across gateway restart; credentials never appear in rendered pages or screenshots.
- [ ] Optional metrics endpoint is loopback-only unless explicitly configured otherwise; verify unavailable metrics do not break configuration access.

## Runtime and failure scenarios

- [ ] Observe startup readiness and clean termination; distinguish process health from actual radio activity.
- [ ] Exercise a busy or unavailable receiver and capture useful diagnostics.
- [ ] Where enabled, exercise APRS-IS/MQTT disconnection and recovery with authorized services or local fixtures.
- [ ] Confirm bounded queue/drop behavior using synthetic traffic, not congestion on the air.
- [ ] For licensed TX setups only, verify PTT, isolation, appropriate dummy-load testing, and safe shutdown before unattended transmission. Skip if no validated hardware.

## Evidence for a release

Record commands, commit, hardware, duration, input source, observed outcome, and limitations. Capture actual sanitized screenshots from the running UI. Run the documented [performance baseline](performance-baseline.md) and retain raw output; do not infer RF latency or competitive superiority from microbenchmarks. Include failures and skipped scenarios.

This is a test plan, not a report of completed testing. The current installer and operational metric follow-ups must be reconciled with this checklist after they land.
