# Your first software-defined radio

One of the neat things about radio today is that a small USB receiver and a computer can let you explore quite a bit of the spectrum without buying a separate radio for every mode.

SDR means software-defined radio. The hardware samples a signal and software does some of the work that traditionally required dedicated radio circuitry: filtering, tuning and demodulation. An RTL-SDR is an inexpensive receive-only example. Not every SDR can transmit, and a normal RTL-SDR can't.

## What you'll need

A supported receiver, an antenna suitable for the band, a computer, and software that works with your device. You don't need a fancy antenna to begin, but the antenna and its placement matter enormously. Start with strong local signals and learn what the spectrum display looks like before hunting weak packets.

For a local APRS station, Sparky uses rtl_fm to produce audio and multimon-ng to decode it. OpenWebRX+ is another option, publishing already-decoded reports over MQTT. Follow the [operator quick start](../operator-quickstart.md) for your chosen backend.

## A few things that might trip you up

- More gain isn't always better. Strong nearby signals can overload a receiver.
- Sample rate, frequency correction and audio levels all matter.
- A USB device appearing in the operating system doesn't prove your account has permission to use it.
- Don't connect a transmitter output to an SDR input. Protect the receiver from nearby RF power.

Start small. Get one signal decoded. Change one setting at a time. That's how you learn what actually made a difference.

Further reading: [rtl-sdr project](https://osmocom.org/projects/rtl-sdr/wiki/Rtl-sdr) and [multimon-ng](https://github.com/EliasOenal/multimon-ng).
