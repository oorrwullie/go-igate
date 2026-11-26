# Nothing's working. Now what?

We've all been there. The trick is to stop changing five things at once and figure out which part of the chain isn't doing its job.

## Work from the antenna toward the destination

| Symptom | First thing I'd check |
|---|---|
| SDR not detected | USB cable, power, port, and whether the OS sees the dongle |
| Device busy | Another decoder or kernel driver already owns it |
| Permission denied | Correct device access for the actual service account; use documented udev rules rather than making all USB devices writable |
| No signals | Antenna connection, suitable band, frequency, gain, nearby activity |
| Signal but no APRS decode | FM audio, sample rate, decoder mode, frequency correction and overload |
| Decodes locally but no map update | Gateway enabled, credentials, network connection and service policy |
| No aircraft positions | Decoder support, antenna, line of sight, message type and location decoding |
| UI isn't showing live status | Metrics endpoint enabled separately on the gateway and correct loopback URL on UI |

On Linux, `lsusb` can confirm USB enumeration and `rtl_test` can help test an idle RTL-SDR. Stop any active decoder before testing the same dongle. `journalctl -u sparky.service` can help diagnose an installed systemd unit; use the actual unit name on your machine. Check its logs too if output is redirected.

## Don't confuse health with results

A running process isn't proof of good RF reception. Neither is a successful socket write proof of remote ingestion. If the channel is quiet, zero decoded packets may be perfectly normal.

Start with a known signal or known recording, isolate each stage, and write down what changed. When asking for help, include hardware, OS, relevant settings and sanitized logs. Never paste passcodes, API keys or passwords.

For Sparky-specific administration, see the [operator quick start](../operator-quickstart.md) and [observability guide](../observability.md).
