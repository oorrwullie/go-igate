Receive, decode, log, upload [APRS](http://www.aprs.org/) packets using low cost [RTL-SDR](http://osmocom.org/projects/sdr/wiki/rtl-sdr) devices.

This project is a rewrite of [Ionosphere](https://github.com/cceremuga/ionosphere) with a number of bug fixes and simplifications.

## Requirements

To run Ionosphere, the following are required.

* An RTL-SDR compatible device.
* [rtl_fm](http://osmocom.org/projects/sdr/wiki/rtl-sdr)
* [multimon-ng](https://github.com/EliasOenal/multimon-ng)

## Usage

* Make sure all software in the Requirements section is installed.
* Ensure your RTL-SDR device is connected.
* Edit `config/config.yml` to match your needs.
  * If configured for automatic beaconing, you may edit the `comment` element to include a latitude, longitude, and symbol.
  * You may find additional documentation on the [APRS protocol](http://www.aprs.net/vm/DOS/PROTOCOL.HTM) and [symbols](http://www.aprs.org/symbols.html) useful for custom comment formats.

## Security and Privacy

**The Automatic Packet Reporting System (APRS) is never private and never secure.** As an amateur radio mode, it is designed solely for experimental use by licensed operators to publicly communicate positions and messages. Encryption on amateur radio frequencies is forbidden in most localities. As such, **connections to APRS-IS are also unsecured and only intended for licensed amateur radio operators.**

## Contributing

You are welcome to contribute by submitting pull requests on GitHub if you would like. Feature / enhancement requests may be submitted via GitHub issues.

## License

MIT license, see `LICENSE.md` for more information.
