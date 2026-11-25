# Sparky documentation

Welcome! You don't need to know everything about radios or Linux to get started. Pick the thing you want to do, and we'll work from there.

## I want to run Sparky

- [Operator quick start](operator-quickstart.md): build, run, and administer your station.
- [Example configuration](../config.yml.example): the versioned YAML example. Review the transmit, digipeater and iGate settings before use.
- [Web administration](web-config.md): the separate authenticated configuration UI.
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
- [Repository contribution instructions](../README.md#contributing).

This index describes the current repository. Don't assume features in an open pull request are already installed on your station.
