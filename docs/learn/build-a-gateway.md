# Build your own radio gateway

Sparky isn't magic. It's a collection of fairly understandable pieces working together. You can build a little version yourself and learn a lot in the process.

~~~text
antenna -> receiver -> audio/IQ -> decoder -> parser
                                               |
                                        duplicate filter
                                               |
                                        bounded routing
                                         /           \
                                    local log      network
~~~

## Start without a radio

Create a text file with a few made-up TNC2 monitor lines, including a repeated frame. Write a small program that reads one line at a time, splits the addressing header from the information field at the first colon, and prints it. Preserve the information field exactly, including trailing spaces.

Add a small bounded duplicate cache with an expiry window. Test repeated packets, malformed lines and cache expiry. Keep packet identity separate from any display formatting; don't confuse deduplication with RF collision handling.

## Add a decoder

Run the [receive-only APRS experiment](first-aprs-station.md) and feed decoder output to your program. Remove only the decoder's known prefix/separator and record terminator. Don't shell-evaluate received text: radio packets are untrusted input.

Next, try sending a copy of a synthetic report to a private MQTT topic. A bounded queue and a defined overflow policy help keep a slow destination from holding up reception. Count accepted and dropped deliveries separately. One packet delivered to two subscribers is two deliveries, not two airframes.

## Going beyond the workbench

A real iGate needs protocol-aware filtering, your own authorized APRS-IS credentials, connection recovery, and appropriate local operating practices. Don't just forward every decoded line to the internet. Internet-to-RF gating and digipeating additionally require a licensed station, tested RF hardware and careful rules to prevent loops and congestion.

If you want to see one approach to this problem, browse [Sparky's source](../../) and [performance notes](../performance-baseline.md). Or use [Dire Wolf](https://github.com/wb2osz/direwolf) as another reference. You don't have to use Sparky to learn any of this.

The fun part is that each piece can be tested on its own before connecting it to the next one.
