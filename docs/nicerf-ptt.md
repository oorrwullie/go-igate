# Simulated NiceRF PTT sequencing

The `internal/nicerf.PTTController` is a **simulation-ready sequencing
component**, not a GPIO driver and not an RF transmitter backend. Nothing in
this PR changes the Sparky application's live transmit path.

## Intended lifecycle

1. Construct a controller with a `PTT` implementation and a `FramePlayer`.
2. After hardware initialization and safety checks, explicitly call `Arm(ctx)`.
   Arming requests `SetTransmit(false)` to establish receive/off before
   authorizing transmission. Construction itself never enables transmit.
3. Call `Transmit(ctx, frame)` to assert transmit, play one complete frame,
   and then request release back to receive/off. Calls are serialized; a
   queued caller can time out or be canceled without affecting the active frame.
4. `Close()` cancels ongoing playback, waits for it to end, makes one final
   request to release PTT, and is idempotent.

The `PTT` driver receives **logical** `enabled` rather than raw voltage.
For the documented SA818S/SA868S pins, TX means PTT driven low and receive
means PTT released. A future GPIO adapter must implement those electrical
semantics, including a safe startup default. A failed deassertion faults the
controller and prevents further attempts to transmit. Fault recovery requires
a new controller after inspecting the hardware state; it is not automatic.

`FramePlayer.PlayFrame` must block until sample playback **actually finishes**,
not merely until samples enter an ALSA/PortAudio buffer. If that contract
cannot be met, the integration must provide an explicit completion signal.
The module's mic input is analog audio; serial AT commands do not send AX.25
packets.

## Scope boundaries and safety

- No UART, GPIO, relay or antenna control is implemented here.
- No key-up lead time or tail time is selected; both require measurement.
- Shutdown requests PTT release with a short independent cleanup timeout.
  It cannot guarantee release if a GPIO adapter fails or the operating system
  or Pi loses power. `Close` may wait if an adapter violates the required
  context-cancellation contract.
- **Software alone cannot keep the SDR safe** on a shared antenna. RF switching
  and isolation require a hardware design and measured verification.
- The actual GPIO circuit should default to RX/PTT inactive through external
  electrical biasing even while the Pi boots, reboots or crashes; follow each
  module revision's absolute ratings and pin polarity.
- Hardware interlocks, frequency acceptance at 144.390 MHz, audio deviation,
  output filtering and transmit permissions must be verified before RF tests.

All tests use fake gate and player interfaces. No real hardware is required.
