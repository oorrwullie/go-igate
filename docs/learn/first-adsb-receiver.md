# Let's listen to some airplanes

This is a fun project because you can start seeing results with very little equipment, especially if you have a reasonably clear view of the sky.

## Parts

- A computer or small Linux board.
- A receive-only SDR capable of 1090 MHz.
- An antenna intended for 1090 MHz and a short, suitable feedline.

The inexpensive whip that came with a dongle may work, but an antenna better suited to 1090 MHz can make a big difference. There's no need for a transmitter.

## Choose a decoder

Use an ADS-B decoder such as [readsb](https://github.com/wiedehopf/readsb) or an [OpenWebRX+](https://github.com/luarvique/openwebrx) installation supporting ADS-B. Follow its installation instructions for your OS and hardware. Start with the local decoder and display; don't sign up for a network feed just to test the receiver.

Some decoders report raw Mode S messages, some show decoded aircraft, and some offer a local map. Not every message contains a location. Aircraft positions may require more than one message or a suitable reference position.

## Try an experiment

Run the receiver for a while, note the number of reports and the approximate coverage, then try a different antenna placement. Keep other settings the same. A clear path to the horizon often matters more than buying an amplifier.

If you want to integrate the resulting reports with Sparky, the currently supported route is OpenWebRX+ decoded ADS-B reports over MQTT, optionally forwarded to map.flights. See the [configuration instructions](../../README.md#receiving-aprs-from-openwebrx). Sparky itself isn't the ADS-B demodulator.

Keep this a receive-only hobby experiment, and don't use a hobby map for flight navigation or safety decisions. Check local laws before collecting or redistributing radio data.
