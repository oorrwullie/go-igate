# Stage 4, slice 2: read-only terminal-style preview

> **Historical implementation note:** this document describes an earlier incremental UI stage, not the current feature set. For shipped behavior, setup, authentication, restart, and live status, use the [operator quick start](operator-quickstart.md).


Run `go run ./cmd/sparky-config --config ./config.yml` from the installation
working directory, then browse to `http://127.0.0.1:8080/` **on that machine**.
For remote administration, use an SSH local port forward instead of exposing
the port to the LAN or Internet. This standalone preview is **not** automatically
started by the normal Sparky service.

The server intentionally binds only to a literal loopback IP, restricts HTTP
Host values to loopback/localhost, and sets no-store, CSP, and framing headers.
The template exposes only an explicit non-secret list of configuration fields.
It does not show passwords, passcodes, station keys or raw YAML; debug messages
must not log them either.

The mock-up's Save and Restart buttons are currently disabled. The live page
accurately reflects the software's existing config types; there are no imaginary
NiceRF frequency/power settings. This PR adds no mutation endpoint, login,
restart access, privilege escalation or broad web framework.

Future PRs must add proper access controls and CSRF protection before enabling
configuration modifications or restarts. Keep YAML authoritative; writes require
validation, optimistic revision checks, and an explicit save. Restart must be a
separate, narrowly authorized action, never arbitrary shell execution.
