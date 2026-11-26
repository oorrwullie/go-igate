# Changelog

Changes worth knowing about will be recorded here as releases are prepared. This is a curated operator-facing summary, not a reconstructed or complete historical release log.

## Unreleased

### Added
- Separate authenticated configuration UI with supported field editing and explicit opt-in fixed-target restart.
- Optional process liveness, readiness and Prometheus metrics, with a bounded read-only runtime status snapshot.
- OpenWebRX+ decoded APRS input and optional embedded local MQTT broker; decoded ADS-B forwarding to map.flights.
- Performance measurement harness and operator quick start.

### Changed
- Repository, module, executable and service identity renamed from go-igate to Sparky.
- APRS packet filtering, duplicate handling, bounded distribution, shutdown and cache persistence improved over the course of development.
- Documentation expanded with architecture, learning guides and contributor resources in the documentation PR.

### Important notes
- Configuration changes require restart; the UI is separate and does not hot-reload the gateway.
- The gateway uses external decoders for local APRS and OpenWebRX+ ADS-B input. Native RF module control is not yet implemented.
- Review [migration guidance](docs/upgrade.md) before changing an existing installation.

For authoritative history see [GitHub releases](https://github.com/oorrwullie/sparky/releases), [commits](https://github.com/oorrwullie/sparky/commits/main/) and [merged pull requests](https://github.com/oorrwullie/sparky/pulls?q=is%3Apr+is%3Amerged). No version number or release date is assigned here until a release is actually tagged.
