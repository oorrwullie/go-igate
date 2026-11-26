# What does an iGate actually do?

An internet gateway (iGate) is the bridge between APRS on the radio and APRS-IS, the internet distribution network. It doesn't have to transmit anything over RF to be useful.

Think of it this way:

~~~text
Radio -> receiver -> decoder -> iGate -> APRS-IS -> applications/maps
~~~

A receive-only iGate can hear packets and forward eligible ones to APRS-IS. A bidirectional iGate can also transmit certain eligible messages back to radio stations it has recently heard. That's not the same as blindly repeating everything from the internet over the air. Doing that would be a very effective way to make everybody's radios unhappy.

## Build up to it

1. Get your receiver decoding local APRS packets reliably.
2. Learn the local network practices and obtain your own amateur callsign and appropriate APRS-IS credentials before operating an iGate.
3. Configure the receive-only RF-to-IS gateway using the [operator guide](../operator-quickstart.md) and version-matched [example YAML](../../config.yml.example). Leave RF transmit, digipeating, message gating and beaconing off until you intentionally need them.
4. Check both local logs and APRS-IS visibility. Seeing a packet in a log is not the same as proving it reached the destination.

If you later add internet-to-RF message gating, you'll need a licensed station, suitable transmitter/PTT hardware, and an understanding of the eligibility rules. Keep your credentials private.

Further reading: [APRS-IS](https://www.aprs-is.net/) and [Sparky's APRS-IS message gating notes](../../README.md#aprs-is-message-gating).
