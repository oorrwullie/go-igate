# Sparky documentation

Welcome! You don't need to know everything about radios or Linux to get started. Pick the thing you want to do, and we'll work from there.

## I want to run Sparky

- [Operator quick start](operator-quickstart.md): build, run, and administer your station.
- [Configuration reference](configuration.md): YAML keys, defaults, dependencies, CLI and environment settings.
- [Hardware compatibility](hardware.md): supported integrations and validation status.
- [Upgrade and migration](upgrade.md): backups, safe transitions and rollback.
- [Troubleshooting](troubleshooting.md): common problems and practical checks.
- [Known limitations](limitations.md): current boundaries and unsupported features.
- [Example configuration](../config.yml.example): the versioned YAML example. Review the transmit, digipeater and iGate settings before use.
- [Web administration](operator-quickstart.md#2-start-the-separate-configuration-ui): the separate authenticated configuration UI. Historical design notes are in [web-config.md](web-config.md).
- [Deployment](web-config-deployment.md): running the admin UI as a service.
- [Restart controls](web-config-restart.md): applying changes without granting broad privileges.
- [Observability](observability.md) and [runtime status](web-config-runtime-status.md): what the gateway can actually tell you.

## I want to learn about radio

Start with the [learning library](learn/README.md). It's for curious people, including those who haven't earned their callsign yet. The learning guides explain the ideas behind Sparky and how to experiment safely. They aren't a replacement for the version-matched configuration and deployment instructions above.

## Maintainers

[Wiki publishing guide](wiki-publishing.md): create a learning hub without maintaining duplicate documentation.

## I want to work on Sparky itself

- [Architecture](architecture.md): system diagram, packet flow, lifecycle and trust boundaries.
- [Engineering decisions](engineering-decisions.md): design rationale and trade-offs.
- [Release acceptance checklist](acceptance-testing.md): actual hardware and installation validation.

- [Performance baseline](performance-baseline.md): how to measure performance without making unsupported comparisons.
- [Contributing to Sparky](../CONTRIBUTING.md): everyone is welcome, from first-time contributors to experienced radio and software engineers.
- [Issue templates](https://github.com/oorrwullie/sparky/issues/new/choose) and [security reporting](../SECURITY.md).
- [Code of conduct](../CODE_OF_CONDUCT.md), [contributors](../CONTRIBUTORS.md), and [changelog](../CHANGELOG.md).

This index describes the current repository. Don't assume features in an open pull request are already installed on your station.
