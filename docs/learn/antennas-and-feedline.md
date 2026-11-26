# Antennas, gain and feedline (without the magic)

If I were trying to improve a little receiving station, the antenna is where I'd start. You can spend a lot of money chasing problems that are actually caused by a poor antenna connection or a terrible location.

## Wavelength and antenna size

Radio waves have a wavelength. Divide about 300 by frequency in MHz to get the wavelength in meters. A quarter-wave radiator is roughly a quarter of that, before accounting for construction and surroundings.

At 144.390 MHz that's about 52 cm. At 1090 MHz it's about 6.9 cm. These are starting dimensions, not a promise that a finished antenna is tuned. A quarter-wave monopole needs a suitable ground plane or counterpoise.

## Gain is not free power

An antenna with gain concentrates sensitivity or transmitted energy in particular directions. An omnidirectional antenna can have gain by flattening its radiation pattern vertically. That may be great for stations near the horizon, but not for every situation.

Receiver gain is different: it amplifies signals and noise, and excessive gain can overload the receiver. Try changing one setting at a time.

## The stuff between the antenna and radio matters

Coax has loss, and loss generally gets worse at higher frequencies. A short feedline can be particularly valuable at ADS-B frequencies. Use appropriate connectors and impedance (commonly 50 ohms for these stations), inspect connections, and weatherproof outdoor connections.

A preamp can't undo losses that happened before it. It can also make overload worse. Start with the antenna and feedline before throwing amplifiers at the problem.

For outdoors, stay well clear of power lines. Use appropriate grounding, lightning protection and safe mounting; disconnect equipment when appropriate and don't work on antennas in storms.

For experiments, compare decoded packets across similar conditions rather than judging everything by a signal-strength number. See [RF power and receiver protection](rf-power-and-protection.md).
