# go-gate APRS Gateway

Receive, decode, log, upload [APRS](http://www.aprs.org/) packets using low cost [RTL-SDR](http://osmocom.org/projects/sdr/wiki/rtl-sdr) devices.

This project is a rewrite of [Ionosphere](https://github.com/cceremuga/ionosphere) with a number of bug fixes and simplifications.

## Description

Using this software you can receive APRS packets from the airwaves, decode them, log them to a file and upload them to the APRS-IS network.

The software is written in Go and uses the [RTL-SDR](http://osmocom.org/projects/sdr/wiki/rtl-sdr) library to receive the packets. The packets are then packets are then passed to 
[multimon-ng](https://github.com/EliasOenal/multimon-ng) for decoding. The decoded packets are then uploaded to the APRS-IS network.

## Features

- Receive APRS packets from the airwaves.
- Decode APRS packets.
- Log APRS packets to a file.
- Upload APRS packets to the APRS-IS network.
- Configurable RTL-SDR device.

## Getting Started
First you need to install the dependencies. Make sure you are running the latest version of Go or at least the version specified in the go.mod file.

### Dependencies
To run Go-iGate, the following are required.

* An RTL-SDR compatible device.
* [rtl_fm](http://osmocom.org/projects/sdr/wiki/rtl-sdr)
* [multimon-ng](https://github.com/EliasOenal/multimon-ng)
* [alsa-utils](https://alsa-project.org/wiki/Main_Page) (`aplay`) when using the soundcard transmitter
 
- Go version go1.22.5

## Usage
### Installing with make

#### Build the application
`make build`

#### Run the application
`make run`

### Installing the service with make
`make install-service`

### Running the application for testing

```bash
go run main.go
```

### Log rotation

Systemd writes APRS logs to `/var/log/aprs.log` by default. Install the logrotate policy to keep the file from growing unbounded:

1. Copy `scripts/aprs.logrotate` to `/etc/logrotate.d/aprs` (requires sudo).
2. Adjust thresholds (e.g. `size`, `rotate`) if your environment needs different retention.
3. Run `sudo logrotate -f /etc/logrotate.d/aprs` once to verify the policy.

The provided config rotates whenever the log hits 50 MB, keeps 10 compressed archives, skips missing/empty logs, and uses `copytruncate` so the running service keeps logging.

### Beacon configuration tips

- A home fill-in digipeater should advertise itself with the WIDE1-1 overlay. Use the alternate table digipeater symbol paired with the overlay character `S` (e.g. `=latNSlonW#...`) to match [APRS Protocol Reference v1.0.1, Chapter 2](http://www.aprs.org/doc/APRS101.PDF).
- APRS expects fixed stations to be visible within a 10–30 minute “net cycle.” Configure the primary RF beacon (your wide path) around 30 minutes and add a secondary direct beacon near 10 minutes to refresh local maps without flooding the network.
- In `config.yml` the `additional-rf-beacons` list lets you schedule those direct packets. Leave the `path` empty to transmit truly direct; adding text like `DIRECT` creates a literal hop and is not recommended.
- Disable APRS-IS (`disable-tcp: true`) if you only want RF packets on aprs.fi. Re-enable it with a conservative `is-interval` when Internet visibility is important.
- After editing the configuration, run `make` to rebuild the binary and restart the `aprs` service so the new schedule takes effect.

## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

Distributed under the MIT License. See `LICENSE` for more information.

## Contact
Project Link: [https://github.com/oorrwullie/go-igate](https://github.com/oorrwullie/go-igate)
