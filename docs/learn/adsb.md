# Listening to airplanes with ADS-B

If you've ever watched aircraft move around a flight-tracking map, you might be surprised how much of that data can be received with a little USB radio.

ADS-B stands for Automatic Dependent Surveillance–Broadcast. Aircraft broadcast information such as identity, position, altitude and velocity. Not every report contains every field. A common ADS-B downlink frequency is 1090 MHz; 978 MHz UAT is also used in the United States.

## Give it a try

A receiver capable of 1090 MHz, a suitable antenna, and a decoder are a good starting point. Antenna placement and a clear view of the sky make a difference. You do not need to transmit anything, and this isn't an invitation to transmit on aviation frequencies.

Sparky currently accepts already-decoded OpenWebRX+ ADS-B reports over MQTT and can forward them to map.flights. It does not decode raw ADS-B samples itself. Read the [OpenWebRX+ integration instructions](../../README.md#receiving-aprs-from-openwebrx) before setting up a feed; use your own credentials and keep them private.

A successful MQTT send doesn't establish that an aircraft appeared on a remote map. Check the destination independently. Treat hobby receiver data as informational, not as a tool for aviation navigation or safety decisions.
