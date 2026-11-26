# RF power: bigger isn't always better

A little receiver can hear remarkably weak signals. That also means a transmitter nearby can make its life rather miserable.

## Think in decibels

Doubling power is about 3 dB. Ten times power is 10 dB. One watt is 30 dBm, and 100 mW is 20 dBm. A transmitter output and an SDR input aren't remotely the same kind of signal level.

Never connect a transmitter output directly to a receiver input. Use a suitably rated dummy load for transmitter tests and a properly designed attenuation/isolation arrangement for any coupled measurement. Consult the receiver's absolute maximum input specification, not just its normal operating range.

## When you have one antenna

A shared-antenna station needs an RF path that isolates the receiver during transmission, with suitable power ratings, isolation, filtering and safe sequencing. Switching off the receiver in software does not physically protect its input. Neither does simply assuming two nearby antennas are far enough apart.

PTT (push to talk) is the signal that tells a radio to transmit. For an automated station, the design should prevent RF being emitted before the antenna path is safe, and return to a safe state if control fails. Verify isolation and sequencing on a bench before connecting sensitive equipment.

## Start small

You don't need to build a shared-antenna transceiver to run Sparky. A receive-only SDR is a great first project. A licensed operator can add a separate external radio and suitable audio/PTT interface later, following its specifications and local rules.

Native transmitter-module control and a self-contained shared-antenna appliance are future Sparky projects, not a tested design we're publishing today.
