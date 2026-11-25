# Known limitations

Here's what Sparky does not claim to do today. We'd rather set the right expectations than have someone buy hardware or deploy a station based on a feature we haven't finished.

- Local capture and OpenWebRX+ MQTT reception are alternatives, not a multi-receiver engine.
- Local APRS uses rtl_fm or sound-card audio and multimon-ng; native Go demodulation is not implemented.
- ADS-B requires decoded OpenWebRX+ reports; Sparky does not decode raw ADS-B RF samples.
- RF transmission uses sound-card audio and a suitable external radio; native RF module control and shared-antenna appliance hardware are not shipped.
- The embedded MQTT broker is local and anonymous, not a public general-purpose service.
- Additional RF beacon schedules are parsed but ignored; only the primary RF schedule is supported.
- The nested igate.aprsis.enabled flag is not a reliable connection switch; disabling parent igate.enabled prevents iGate creation.
- The admin UI edits a subset of YAML and does not hot-reload the running process. Restart is opt-in and supervisor-specific.
- Health/readiness indicate process state, not RF reception or broker availability. Queue deliveries are not unique airframes. Metrics have no built-in historical store or automatic dashboard.
- The optional UI metrics client only accepts literal loopback endpoints; a separate Kubernetes Pod cannot directly use that loopback connection.
- Bounded queues may drop data during overload. Deduplication/cache snapshots are best-effort, not a durable packet archive.
- No comprehensive competitive RF accuracy, latency or platform performance claims have been established.
- No universal binary portability or hardware certification is claimed. See [hardware compatibility](hardware.md).

The roadmap may include improvements to these boundaries, but an open PR or a future plan is not a released feature.
