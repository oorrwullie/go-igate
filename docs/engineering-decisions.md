# Engineering decisions and trade-offs

This is a concise explanation of the design choices visible in Sparky's code. It is not a claim that every decision was made formally in advance.

| Decision | Why | Trade-off |
|---|---|---|
| External rtl_fm and multimon-ng | Reuse established capture and decoder tools; keep the Go gateway focused on routing | Subprocess lifecycle, IPC and deployment dependencies |
| Bounded nonblocking PubSub | A slow subscriber cannot indefinitely hold up capture | Drops under overload; consumers need visibility and realistic queue sizing |
| Input and protocol-specific deduplication | Reduce unnecessary forwarding and RF repeats | Cache identity and expiry matter; duplicates may sometimes be meaningful |
| Separate admin process | Configuration access survives a gateway restart | Two services to deploy and secure |
| Save then explicitly restart | Avoid implying a saved file is live; predictable startup validation | Configuration changes are not immediate |
| Fixed-target supervisor adapter | Avoid arbitrary process control from the web interface | Platform-specific setup and narrow privileges required |
| Optional loopback metrics | Simple operational visibility with no exporter I/O in packet handling | No history, no remote dependency health guarantee |
| Embedded broker for local integration | Remove an optional external broker hop for OpenWebRX+ | Anonymous local broker must not be treated as a public service |
| Hardware backend abstraction for TX | Keep routing and scheduling separate from hardware-specific playback | Dispatch alone cannot prove successful RF emission |

## Reviewable evidence

- [PR #49](https://github.com/oorrwullie/sparky/pull/49): fixed-target restart support.
- [PR #50](https://github.com/oorrwullie/sparky/pull/50): independent admin deployment.
- [PR #51](https://github.com/oorrwullie/sparky/pull/51): optional observability and health.
- [PR #52](https://github.com/oorrwullie/sparky/pull/52): bounded read-only runtime snapshot.
- [PR #53](https://github.com/oorrwullie/sparky/pull/53): initial performance measurement harness.
- [PR #55](https://github.com/oorrwullie/sparky/pull/55): operator UX and quick start.

## Deliberate boundaries

Native demodulation, integrated transmitter hardware, historical dashboards and broader protocol support are not necessary to validate the current gateway. Measure before optimizing, and distinguish clean-audio correctness from over-the-air sensitivity and end-to-end latency. See [architecture](architecture.md) and [performance baseline](performance-baseline.md).

Never let what Sparky could become interfere with making what it already is exceptional.
