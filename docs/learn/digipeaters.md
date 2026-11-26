# Digipeaters: giving a packet a little more reach

A digipeater hears an eligible digital radio packet and retransmits it over the air, usually after applying path rules. An iGate, on the other hand, moves packets between radio and the internet.

Why bother repeating packets? Terrain, antenna height and buildings can get in the way. A well-placed digipeater may hear both a mobile station and a more distant receiver.

## What are those WIDE things?

APRS paths can request digipeater hops. A fill-in digipeater commonly responds to WIDE1-1 under local policy. Wider-area digipeaters often participate in WIDEn-N paths. The path is a request, not a guarantee that a specific number of stations will repeat it.

More hops aren't automatically better. Every repeat takes airtime, and two stations transmitting together may collide. Coordinate with your local APRS community before adding another digipeater.

## Before you put one on the air

Get your [license](amateur-radio-license.md), check local practices, and make sure your transmitter, antenna, PTT and audio chain work. Test into a suitable dummy load or controlled setup first. Review Sparky's version-matched [configuration example](../../config.yml.example), including alias patterns, duplicate filtering and transmit settings. Don't enable a digipeater just to see what happens on a busy channel.

Start with reception, then deliberate transmission, then unattended operation only after validating shutdown and RF behavior.
