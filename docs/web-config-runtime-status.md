# Read-only gateway runtime status

The authenticated `sparky-config` overview can optionally show the live
gateway's readiness, uptime, and admitted/dropped PubSub subscriber deliveries.
This functionality is deliberately **read-only**. It never creates a metrics
exporter or adds work to the RF or MQTT hot path.

The gateway's observability endpoint is available in the same repository;
both processes need their opt-in configuration. See [operator quick start](operator-quickstart.md).

## Enable

On the gateway process:

```sh
(cd bin && SPARKY_OBSERVABILITY_ADDR=127.0.0.1:9091 ./sparky)
```

On the separately managed configuration UI:

```sh
SPARKY_CONFIG_PASSWORD='your-long-secret-password' \
  ./bin/sparky-config --config ./bin/config.yml \
  --metrics-url http://127.0.0.1:9091/metrics
```

The UI fetches a bounded (8 KiB), short-deadline (400 ms) snapshot only when
an authenticated administrator loads its overview page. Missing/unavailable
or invalid metrics are shown as **unavailable** while the saved configuration
remains accessible. There is no polling, browser-to-gateway request, automatic
refresh, history database, JavaScript dashboard, or request on the packet
processing path.

The metrics target is fixed at startup and must use an HTTP literal loopback
IP and exactly `/metrics`. Arbitrary URLs and redirects are rejected. The
client does not use environment HTTP proxies and never forwards the admin's
Basic auth password to the metrics endpoint.

In Kubernetes, this loopback option works when the gateway and UI share a
Pod network namespace (sidecar deployment). A separate UI Pod does not share
the gateway's loopback network; secure cross-Pod status access requires a
separately reviewed design. Do not disable validation to fetch arbitrary
Pod URLs.

The counters represent **admitted and dropped PubSub subscriber deliveries**,
not unique RF packets or received airframes. The dashboard may eventually show
historical rates with a bounded sampling policy or an external metrics store.

The radio activity section shows optional operational counters when supplied by the gateway. Older gateways still show the original four metrics. Unavailable or invalid optional values are omitted; a zero last-input timestamp displays Never. Connection and dispatch indicators are local observations, not proof of RF or remote delivery.
