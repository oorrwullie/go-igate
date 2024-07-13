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