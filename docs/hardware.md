# Hardware and platform compatibility

We want Sparky to work well on inexpensive equipment, not just powerful development computers. This is a compatibility guide, not a certification list. A supported code path does not mean every device has been tested.

| Hardware / platform | Integration | Evidence status |
|---|---|---|
| RTL-SDR compatible receiver | rtl_fm to multimon-ng | Implemented; exact dongle/OS combinations need recorded validation |
| External FM receiver and USB sound card | PortAudio input to multimon-ng | Implemented; device-level audio and sample-rate testing required |
| OpenWebRX+ receiver | Decoded APRS and ADS-B over MQTT | Implemented; validate versions and reports |
| External radio with audio interface | AFSK1200 output via PortAudio or ALSA aplay | Implemented; verify PTT/VOX, levels and RF independently |
| Raspberry Pi Zero 2 W | Linux SBC candidate | Target for acceptance and benchmarks; no published resource measurements yet |
| Raspberry Pi 4/5 and Orange Pi 5 Plus | Linux SBC candidates | Hardware-specific validation pending |
| Linux amd64 | GitHub Actions Ubuntu build/test | Automated software evidence, not RF certification |
| Linux arm64 / armhf | Go + platform PortAudio dependencies | Build and hardware acceptance pending |
| macOS | Go / PortAudio; launchd restart adapter | Adapter implemented; real deployment and audio validation pending |
| Kubernetes | Process probes and supported restart approach | Deployment-specific validation pending |
| Native VHF TX modules | Not currently integrated | Planned; do not buy hardware assuming native support |
| Raw ADS-B SDR decoding | External decoder required | Not implemented in Sparky |

## Before choosing equipment

Check frequency coverage, connector and antenna suitability, OS drivers and device permissions. For SDR, make sure rtl_fm supports the receiver. For audio, verify the selected device and sampling behavior, and prevent unwanted transmit activation.

Never connect a transmitter output directly to an SDR input. See [receiver protection](learn/rf-power-and-protection.md).

## Share your results

Use the [hardware/station issue template](https://github.com/oorrwullie/sparky/issues/new/choose). Include board, CPU architecture, OS, receiver model, decoder versions, Sparky commit, configuration mode, workload and observed results. Sanitize secrets and private station information. Reports of successful installations are welcome.

We can turn reproducible reports into a verified compatibility matrix over time. See the [release acceptance checklist](acceptance-testing.md).
