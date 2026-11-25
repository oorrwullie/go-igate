# Why more power and more repeats aren't always better

APRS is a shared radio channel. Imagine several people talking at once on the same frequency. A receiver may have a hard time separating them. Digital packets can collide too.

A stronger overlapping signal may sometimes capture an FM receiver, but that doesn't mean the weaker signal was decoded. More power can improve a particular link while making reception harder for other stations. More beaconing and more digipeater hops also consume airtime.

## What can we do about it?

Listen before transmitting, use sensible beacon intervals and paths, coordinate with local APRS operators, and avoid unnecessary repeats. A quiet channel at your antenna doesn't guarantee every other station can hear the same transmitters (the hidden-terminal problem).

Deduplication helps avoid processing or forwarding the same frame repeatedly after reception. It doesn't restore a packet destroyed by an RF collision.

## A little experiment

Start with receive-only observations. Compare packet activity at different times of day, or use offline recordings and a decoder. A clean generated waveform is useful for a sanity check, but it isn't evidence of weak-signal performance. A proper test corpus needs known ground truth and controlled impairments.

See [how digipeaters work](digipeaters.md) and the [Sparky performance baseline](../performance-baseline.md) for more. No on-air collision experiments are needed.
