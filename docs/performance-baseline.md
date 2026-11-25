# Sparky packet-processing performance baseline

This is a **measurement harness**, not a benchmark result or an optimization.
It adds Go microbenchmarks without changing production code, packet handling,
queue behavior, metric collection, or dependencies. The goal is repeatable
before/after evidence for the "Simple. Powerful. Fast." engineering contract.

## Quick reproducible run

From the repository root, with the same compiler and build flags on both
revisions:

```sh
go version
go env GOOS GOARCH GOARM GOAMD64 CGO_ENABLED
go test ./internal/aprs ./internal/pubsub ./internal/observability \
  -run '^$' -bench 'Benchmark(ParsePacket|ParseAndClassify|PositionDecode|Fanout|PublishOneSubscriber|MetricsScrape)' \
  -benchmem -count 7 -timeout 10m \
  | tee sparky-bench-$(date -u +%Y%m%dT%H%M%SZ).txt
```

For comparisons, collect the seven-run output from each revision on the
**same host** under similar load and power/thermal conditions. Go's
`-benchtime=1s` default is adequate for an initial run. Record commit SHA,
Go version, OS/kernel, architecture, CPU governor, thermal state, and any
hardware throttling. Report medians and dispersion, not just the best sample.
Use `benchstat` only if already installed; it is not a project dependency.
Benchmark results are not comparable across different CPUs by ns/op alone.

## Current coverage

| Benchmark | Measures | Does not measure |
|---|---|---|
| `BenchmarkParsePacket` | Raw APRS text → parsed packet, allocations | Demodulation or RF sensitivity |
| `BenchmarkParseAndClassify` | APRS parsed text and type classification | RF acquisition or APRS-IS network |
| `BenchmarkPositionDecode` | Position decode using a pre-parsed packet | Full APRS pipeline |
| `BenchmarkFanoutAdmitted` | One, two, or four subscriber queues; delivery counter overhead included | Real concurrent subscribers or OS scheduling |
| `BenchmarkFanoutSaturated` | Saturated channel drop behavior; no slow-client blocking | Receiver throughput during sustained overload |
| `BenchmarkPublishOneSubscriber` | Existing single-consumer hot-path microbenchmark | An uninstrumented baseline from before O11y |
| `BenchmarkMetricsScrape` | On-demand metric text rendering | RF-side work or HTTP network latency |

**Important:** The PubSub counters count subscriber *deliveries*, not unique
RF frames. Benchmarks use fixed syntactically valid packet strings and
prearranged in-memory subscribers. Successful fanout drains consumers each
iteration so queues do not inadvertently switch into the dropped-packet path.

## What is not yet proven

These benchmarks **cannot** establish that Sparky decodes weaker signals,
wins races with Dire Wolf, or processes more over-the-air packets. Those
claims require identical recorded IQ/audio replay, equal decode settings,
ground-truth valid packet comparison, and end-to-end latency from sample
capture to gate/forward result. Keep the phases distinct:

1. **Text path:** this PR's microbenchmarks and O11y cost assessment.
2. **Audio/DSP path:** repeatable known-signal AFSK1200 recordings and decoder
   comparisons; count correct, false, and missing decodes.
3. **End-to-end path:** packet capture through APRS-IS/destination, including
   queueing and backpressure; record median/p95/p99 latency and drops.
4. **Device runs:** Pi Zero 2 W, Orange Pi 5 Plus, and any supported x86/ARM
   baseline, accounting for external `rtl_fm` and `multimon-ng` process RSS,
   CPU, system load and power/thermal throttling.

Do not claim no O11y overhead: the admitted fanout path includes one additional
atomic increment per successful subscriber delivery. A strict O11y before/after
comparison requires benchmarking the exact pre-O11y commit and current main
with equivalent benchmarks (or a carefully isolated no-instrumentation variant).
This PR establishes the measurement method, **not** that comparison.

## Regression policy

Do not gate GitHub CI on a single noisy ns/op threshold. Continue to gate
correctness and race safety in CI, then run repeated before/after benchmarks
for hot-path changes. Investigate persistent regressions in allocations,
median and p99 latency, dropped packets, or CPU/RSS, rather than optimizing
based on one sample or an arbitrary score.
