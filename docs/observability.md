# Observability foundation (O11y v1)

This feature is intentionally small and performance-first. Sparky does not
export packet contents or callsigns, allocate O11y events, block for a metrics
consumer, make network calls, or write metrics on the packet-processing path.
The only new fast-path operation is one atomic increment for each successful
PubSub subscriber delivery. Scrapes read atomic snapshots independently.

## Enable

By default no listener is created. To opt in on the same host:

```sh
SPARKY_OBSERVABILITY_ADDR=127.0.0.1:9091 ./bin/sparky
curl http://127.0.0.1:9091/healthz
curl http://127.0.0.1:9091/readyz
curl http://127.0.0.1:9091/metrics
```

Available GET/HEAD endpoints:

- `/healthz` returns HTTP 200 for a responsive process, 503 during shutdown.
  It never checks remote APRS-IS, MQTT or RF hardware.
- `/readyz` returns 503 before the main Run workers are started or after
  shutdown begins, 200 while running. It is **not** a hardware/packet health
  check. Kubernetes may use it for a basic process-readiness probe.
- `/metrics` exports Prometheus text format version 0.0.4: process uptime,
  readiness, admitted PubSub subscriber deliveries, and subscriber delivery
  drops. The counters count **deliveries**, not unique packets or airframes;
  an APRS frame fan-out to two subscribers counts twice.

The observability listener runs within the **gateway** process (not the
separate web configuration admin process) because readiness and counters
describe this specific runtime. It has bounded HTTP timeouts and never
initiates restart, configuration writes, or external dependency probes.

## Networking

For safety, the listener binds only to a **literal loopback IP** unless
`SPARKY_OBSERVABILITY_ALLOW_REMOTE=1` is also set. When exposed on a Pod IP
or `0.0.0.0`, endpoints are **unauthenticated**, so restrict access via
Kubernetes NetworkPolicy, host firewall or a trusted reverse proxy. Avoid
public exposure; metrics may still reveal operational activity patterns.

Example for a Kubernetes Pod with an appropriately restricted network policy:

```yaml
env:
  - name: SPARKY_OBSERVABILITY_ADDR
    value: "0.0.0.0:9091"
  - name: SPARKY_OBSERVABILITY_ALLOW_REMOTE
    value: "1"
livenessProbe:
  httpGet:
    path: /healthz
    port: 9091
readinessProbe:
  httpGet:
    path: /readyz
    port: 9091
```

Do not define readiness by whether third-party networks respond: a temporary
APRS-IS or MQTT outage should not induce an unnecessary Kubernetes restart.

## Performance and future dashboard

Benchmark the packet path with:

```sh
go test ./internal/pubsub -run '^$' -bench BenchmarkPublishOneSubscriber -benchmem
go test ./internal/observability -run '^$' -bench BenchmarkMetricsScrape -benchmem
```

Prometheus-compatible metrics can later feed a Sparky web UX dashboard or an
external scraper. V1 intentionally does **not** introduce an HTTP metrics
client on RF paths, a polling goroutine, labels derived from packet content,
runtime network dependency probes, an OTel collector, or unbounded retention.

Additional staged counters (deduplicated APRS frames, successful RF-to-IS
gates, TX, ADS-B totals, broker state) should be benchmarked separately and
added only where an atomic update is cheap and semantics remain unambiguous.
