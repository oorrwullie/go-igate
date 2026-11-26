# What's APRS, anyway?

Imagine driving through the mountains with a little radio sending your location every so often. Another radio hears it, and a computer can turn that information into a dot on a map. Pretty cool, right?

APRS stands for Automatic Packet Reporting System. Position reports are probably the best-known use, but APRS can also carry short messages, weather observations, station status and other information.

## What's actually going over the air?

A common setup uses 1200-baud AFSK packet radio on 2 meters. Two audio tones represent digital information. A radio sends the audio over FM; at the other end, a decoder turns it back into data. The AX.25 link-layer frame carries addressing and the information field.

Here's a made-up TNC2 monitor representation (not a packet to transmit):

~~~text
N0CALL-7>APRS,WIDE1-1:>Hello from the workbench
~~~

Before the colon is the source, destination and requested path. After it is the information field, in this case a status report. The text you see isn't a literal recording of the RF signal.

## Try listening first

In much of North America, the common terrestrial APRS frequency is 144.390 MHz. Check your regional band plan elsewhere. A compatible SDR and a suitable antenna are enough to start. Use a local decoder, or a receive-only Sparky setup following the [operator quick start](../operator-quickstart.md). Review the example configuration and explicitly disable TX, digipeating, and iGate operation before your first run.

Don't see a packet right away? That doesn't necessarily mean something is broken. APRS is bursty. Check the frequency, antenna, gain, and whether your area actually has activity.

Once you've seen a packet, learn about [iGates](igate.md) and [digipeaters](digipeaters.md). They aren't the same thing, even if they often live on the same computer.

Further reading: [APRS protocol reference](http://www.aprs.org/doc/APRS101.PDF) and [aprs.fi](https://aprs.fi/).
