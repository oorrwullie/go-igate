# Let's build a little APRS receiving station

You don't need a transmitter, a callsign or a pile of expensive equipment for this one. All we're going to do is listen.

## What you'll need

A Linux computer (an old laptop or Raspberry Pi is fine), an RTL-SDR with support for 2 meters, a suitable antenna, and a USB connection. For this experiment, leave any transmit equipment disconnected.

On Debian or Ubuntu, install the tools:

~~~sh
sudo apt update
sudo apt install rtl-sdr multimon-ng
~~~

Plug in the dongle and run `rtl_test`. Stop it with Ctrl-C when you've confirmed samples are arriving. If it complains about access or a busy device, check the [troubleshooting guide](station-troubleshooting.md). Don't solve a permission problem by running everything as root.

## Let's hear some packets

In North America, try 144.390 MHz (check the local APRS frequency elsewhere):

~~~sh
rtl_fm -f 144.390M -M fm -s 22050 -g 20 -l 0 - | multimon-ng -a AFSK1200 -A -t raw -
~~~

Here the first program tunes and demodulates the FM signal into audio samples. The second looks for packet tones and decodes them. The gain is a starting point, not a universal optimum. Adjust it for your particular receiver and conditions.

You should eventually see lines beginning with APRS. Ctrl-C stops the pipeline. No network service is involved, and nothing is transmitted.

## What if it works?

Congratulations, you've built the receive portion of a packet radio station without Sparky! Now try different antenna placements and compare how many frames you decode over similar periods. Be careful interpreting short tests; traffic comes and goes.

If you'd like to turn it into a managed station, use Sparky's [quick start](../operator-quickstart.md). On the current version, explicitly disable the transmitter, digipeater and parent iGate before starting a local-only experiment. See [building your own gateway](build-a-gateway.md) for an alternative DIY route.
