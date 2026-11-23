# NiceRF SA818S-V / SA868S-V integration notes

Status: protocol-only implementation, **not** hardware-validated. No GPIO, audio
or RF transmission is enabled by the `internal/nicerf` package.

## Sources

- SA818S manual, rev 1.1 (protocol and pin definitions):
  https://xonstorage.blob.core.windows.net/pdf/gnicerf_sa818su_xonlink.pdf
- SA868S manual, rev 1.7 (protocol and pin definitions):
  https://www.nicerf.com/pdf/sa868s-2w-embedded-walkie-talkie-modulev1.7.pdf
- NiceRF official product pages:
  https://www.nicerf.com/walkie-talkie-module/1w-embedded-walkie-talkie-module-sa818s.html
  https://www.nicerf.com/walkie-talkie-module/2w-embedded-walkie-talkie-module-sa868.html

## Confirmed command differences

Both modules document 9600-baud, 8N1, ASCII command/response pairs with CRLF
terminators. Both accept `AT+DMOCONNECT` and `AT+SETFILTER`.

**Important:** `AT+DMOSETGROUP` has the same apparent shape on both models,
but its *first argument differs*:

- SA818S: `GBW`, channel bandwidth; 0 = 12.5 kHz, 1 = 25 kHz.
- SA868S: `TX Power`; 0 = high, 1 = low.

The codec never shares a first-field interpretation between models. The current
SA868S implementation always selects low power. The same module also has an
H/L hardware power-control pad; that is a separate electrical concern.

The current codec intentionally uses no CTCSS/DCS (`0000`) and monitor-mode
squelch (`0`) for the first bench setup. It rejects frequencies outside the
VHF module range and values that cannot be represented at four decimal places
in MHz. It does not round requested frequencies.

Example APRS setup commands (configuration only, **not** authorization to key
PTT or transmit):

```text
SA818S: AT+DMOSETGROUP=0,144.3900,144.3900,0000,0,0000
SA868S: AT+DMOSETGROUP=1,144.3900,144.3900,0000,0,0000
```

## Items requiring hardware verification

- The datasheets specify frequency-grid requirements in 12.5/25 kHz terms,
  while North American APRS is 144.390 MHz. Third-party SA818 APRS projects
  use 144.390 MHz, but actual command acceptance and measured RF frequency on
  **both purchased module revisions** must be checked before on-air operation.
- Verify exact UART voltage levels and pin numbering against the received
  board revision and the manufacturer manual; don't assume Pi GPIO compatibility.
- PTT is active-low, PD is active-high normal operation. H/L **must not be
  driven high**; verify safe default/drive arrangement for both modules.
- The module's MIC_IN accepts analog audio, not serialized AX.25 frames. A
  suitable DAC/audio interface and measured deviation are still required.
- Confirm filter settings preserve 1200/2200 Hz Bell 202 tones and acceptable
  occupied bandwidth. Ensure audio is muted before asserting PTT and after
  release, and allow appropriate TX settling/turnaround in the hardware backend.
- Use a dummy load, measured RF switching/isolation and a regulated supply.
  Do not connect the RTL-SDR to a shared antenna path until receive protection
  is verified; software shutdown is insufficient protection.

## First driver slice

`internal/nicerf` provides model-specific formatting and response validation,
with tests that run without a serial adapter or transmitter. Subsequent PRs
can add serial transport with bounded response reads and timeouts, model
initialization, PTT-safe state machine, and then the audio/RF integration.
