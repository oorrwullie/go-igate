# Come help build Sparky!

One of the things I love about open source is that somebody halfway around the world can notice something I missed, have an idea I hadn't considered, or figure out why something doesn't work on a piece of hardware I've never seen. We all learn something in the process.

You don't have to be an expert in Go, radio, or Linux to help. You don't even need an amateur radio license to contribute code, documentation, or receive-only experiments. If this is your first open-source contribution, you're welcome here.

## There are lots of ways to help

- Found a bug? Tell us what happened, even if you don't know how to fix it.
- Tried Sparky on different hardware? Tell us what worked and what didn't.
- Found something confusing in the documentation? That's a bug worth fixing, too.
- Know something about radio? Help us explain it in a way that makes sense to someone just starting out.
- Enjoy writing code? Fix a bug, add a test, or investigate a performance problem.
- Have an idea? Let's talk about the problem it would solve.

Start with the [open issues](https://github.com/oorrwullie/sparky/issues), or [open an issue](https://github.com/oorrwullie/sparky/issues/new/choose) if you don't see yours. For small fixes like a typo, a pull request is fine without an issue. For a larger feature or architectural change, please discuss the idea first so nobody spends a weekend building something we aren't ready to integrate.

## What we're trying to build

**Simple. Powerful. Fast.**

Sparky should be easy to operate, capable enough to be useful, and quick enough to get out of the way of the radio data. We like inexpensive hardware, predictable resource usage, and software that keeps working when nobody is watching.

A few things we care about:

- Keep changes focused. A small, understandable fix beats a giant rewrite nobody can review.
- Prefer simple solutions. Add a dependency or abstraction when it earns its keep.
- Don't block packet reception on slow consumers, telemetry exporters, or unrelated network operations.
- Keep queues bounded, handle cancellation and cleanup, and be explicit about what happens when something fails.
- Measure performance rather than guessing. If a change touches a hot path, include relevant before/after results when possible.
- Make it easy for someone else to install, understand, and troubleshoot what you've built.

Never let what Sparky could become interfere with making what it already is exceptional.

## Getting the code running

Fork [Sparky](https://github.com/oorrwullie/sparky) on GitHub, clone your fork, and create a branch from the current main branch:

~~~sh
git clone https://github.com/YOUR-GITHUB-USERNAME/sparky.git
cd sparky
git remote add upstream https://github.com/oorrwullie/sparky.git
git fetch upstream
git switch -c fix/describe-your-change upstream/main
~~~

Replace YOUR-GITHUB-USERNAME with your actual username.

You'll need the Go version specified in [go.mod](go.mod), a C compiler, pkg-config, and PortAudio development headers. On Debian or Ubuntu, the following covers the common development dependencies and local decoder tools:

~~~sh
sudo apt update
sudo apt install build-essential pkg-config portaudio19-dev rtl-sdr multimon-ng sox
~~~

On other platforms, use the equivalent packages. See the [operator quick start](docs/operator-quickstart.md) and [architecture guide](docs/architecture.md) for more context.

~~~sh
make build
go test ./...
go test -race ./...
go vet ./...
~~~

Building and testing should not require transmitting RF or connecting to a production broker. Some hardware-specific scenarios need a real device; use local fixtures when possible and tell us what you couldn't test.

To run a local receive-only station, prepare a private configuration following the operator guide. Review it before starting; disable the transmitter, digipeater, iGate, beaconing and unrelated forwarding services. Don't commit your station configuration or credentials.

## Making a pull request

1. Make your change on your branch. Format changed Go files with gofmt.
2. Add a focused test for changed behavior where practical. A regression test that fails before the fix is particularly useful.
3. Run the relevant tests and the broader checks above. For performance-sensitive work, see the [benchmark methodology](docs/performance-baseline.md).
4. Update documentation and examples if operator-visible behavior changes.
5. Review your own diff. Commit with a descriptive message, push to your fork, and open a pull request against main.

Tell us what problem you solved, why you chose this approach, how you tested it, and what remains uncertain. Screenshots help with UI changes. Real hardware reports are valuable, but don't claim testing you haven't performed. Draft PRs are welcome if you'd like feedback while you're working.

We'll review the code, ask questions, and sometimes suggest a simpler approach. That's part of collaborating, not a judgment of you. CI passing is useful evidence, not a substitute for reviewing behavior.

## Radio and security considerations

Receive-only experiments are a great place to start. Follow your local radio rules, obtain the appropriate license before transmitting independently, and test transmit changes with suitable equipment and safe isolation. Please don't test failure modes by creating interference on a shared channel.

Before sharing a config, log, screenshot, packet capture or benchmark fixture, remove passwords, passcodes, API keys and unnecessary personal information, including precise station locations. Keep real RF input topics private. Never publicly post an exploitable security vulnerability; see [security reporting](SECURITY.md).

## Be good to each other

Ask questions freely, explain your reasoning, and treat people with respect regardless of experience, background or skill level. Critique the idea or code, not the person. We don't need to agree on everything to learn from each other.

Thanks for taking the time to help. Even a small correction can make Sparky better for the next person who comes along.
