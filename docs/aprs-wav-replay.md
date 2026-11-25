# Recorded APRS WAV replay: first accuracy baseline

This is a **standalone offline test tool**. It does not modify Sparky,
transmit RF, call APRS-IS, download files, or add work to a packet hot path.

## Purpose

Use exactly the same recorded AFSK1200 audio and expected TNC2 packets to
measure decode correctness and repeatability. The initial decoder is
`multimon-ng` because Sparky currently delegates AFSK1200 demodulation to
it. The evaluator is *not* an end-to-end Sparky benchmark: it exercises the
decoder directly on a WAV file rather than `rtl_fm` or the gateway routing
stack.

## Quick start

Prerequisites: Python 3.10+, multimon-ng, a PCM WAV, and a UTF-8 text file
containing **one expected APRS packet per line**, in monitor/TNC2 notation.
Create an initial known packet with the existing generator:

```sh
make reference-audio
printf '%s\n' 'N7RIX-10>APRS,WIDE1-1,WIDE2-1:=4010.30NS11137.60W#Powered by gophers!' > bin/expected-aprs.txt
python3 scripts/evaluate-aprs-wav.py \
  --wav bin/reference-aprs.wav \
  --expected bin/expected-aprs.txt \
  --repeat 5 --output bin/replay-result.json
```

The reference-audio generator currently needs a local Dire Wolf source
checkout for `gen_packets` and `atest`; do not treat its generated waveform
as a real weak-signal/airwave recording.

The JSON report contains audio metadata, each run's decoded/correct/missed/
extra counts, recall and precision, whole-process elapsed time, and whether
the actual decoded frame multisets agree (regardless of order). Exit code 0 means every expected occurrence matched
without extras in every run; 1 means an accuracy mismatch; 2 means a tool,
fixture or input error.

No packet is truncated, filtered by callsign, or treated as correct by
prefix. Packet comparison is **exact UTF-8 text using a multiset**. Repeated
identical frames in a recording must appear repeatedly in the ground truth.
Order is ignored deliberately. The evaluator removes only LF/CRLF record
terminators and the APRS: prefix with its single optional separator space.
Payload whitespace is significant; do not trim expected or decoded frames.
Invalid UTF-8 decoder output is a tool error, not a replacement-character match.

## Scientific limitations

- **Process elapsed time is not over-the-air detection latency.** It
  includes decoder launch, full-file input and text output. Do not rank
  receivers by this number alone.
- Generated clean audio is a *sanity check*, not proof of weak-signal
  sensitivity or over-the-air RF performance.
- A larger corpus needs recordings with independently validated ground truth,
  controlled signal/noise ratios, packet collisions, signal fading, and false
  decodes. Store provenance, WAV hashes, and decoder build/version in
  published results. The tool does **not** infer ground truth from a decoder.
- Avoid comparing different sample rates, recordings, gains, and demodulator
  options as though they were equivalent.
- Full Sparky-vs-Dire-Wolf comparisons will need a separate adapter for
  Dire Wolf output and end-to-end tests that include capture, IPC,
  deduplication and APRS-IS routing.

## Tests

```sh
python3 -m unittest discover -s scripts -p 'test_evaluate_aprs_wav.py' -v
```

The tests use a synthetic WAV and a fake decoder executable; no SDR
hardware, Dire Wolf or multimon-ng installation is required. This work
belongs in a separate PR from the #53 Go microbenchmark harness.
