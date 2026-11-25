# Troubleshooting Sparky

Start at the earliest stage that isn't working. A healthy process isn't necessarily a healthy radio station, and a quiet channel isn't necessarily a broken receiver.

Before sharing output, remove credentials, exact station locations and other private information. Use the actual service name and configuration path for your installation.

## Build and startup

| Symptom | Check |
|---|---|
| PortAudio headers not found | Install a C compiler, pkg-config and platform PortAudio development headers; on Debian/Ubuntu use portaudio19-dev |
| config.yml not found | Gateway loads from its working directory; inspect systemd WorkingDirectory or run from bin after preparing config |
| YAML parse error | Check indentation, types and durations; compare [configuration reference](configuration.md) |
| rtl_fm or multimon-ng cannot start | Install tools and verify PATH for the actual service account |
| Metrics address already in use | Check listener ownership; change the address deliberately or disable optional metrics |

## Local reception

Stop the gateway before testing a dongle already in use. On Linux, lsusb confirms enumeration and rtl_test checks sample capture. Check the actual account's device permissions; use narrowly scoped udev rules rather than blanket permissions or running the admin UI as root.

Check frequency, antenna, gain, PPM correction, squelch and sample rate. Local APRS uses 22050 Hz audio for multimon-ng. With sound-card capture, enable multimon.receive-diagnostics to see input frames, levels, decoder bytes and dropped buffers. A strong signal can still be distorted, clipped or overloaded.

Use [the independent receive-only experiment](learn/first-aprs-station.md) to isolate reception and decoding. Never troubleshoot by transmitting unauthorized packets.

## Network forwarding

| Symptom | Check |
|---|---|
| iGate startup fails | Parent igate.enabled opens APRS-IS; check station callsign, server, passcode and connectivity. For local-only reception disable the parent |
| Packets decode but don't forward | Check duplicate suppression, APRS packet policy, geographic filter and APRS-IS connection |
| MQTT input is quiet | Check external broker or local embedded listener, matching subscription topic and OpenWebRX+ publishing |
| ADS-B reports not forwarded | Enable OpenWebRX+ input and map-flights, use correct concrete topic, TLS broker and valid station credentials |
| MQTT publish completes but aircraft aren't visible | QoS 0 completion doesn't prove ingestion; check map.flights separately |

Do not publish synthetic data to public networks. Use local fixtures for testing.

## Administration and metrics

| Symptom | Check |
|---|---|
| HTTP 401 | Use admin and the independently configured password |
| HTTP 503 | Confirm valid SPARKY_CONFIG_PASSWORD (16+ characters) and readable configuration |
| HTTP 409 on saving | Reload; configuration was changed externally |
| Save succeeds but no runtime change | Explicitly restart the gateway; saved YAML isn't live configuration |
| Restart missing or fails | Enable only the appropriate fixed-target supervisor adapter; verify narrow privileges |
| Runtime unavailable | Both gateway metrics listener and UI --metrics-url must be configured; use literal loopback /metrics |
| Readiness is green but no traffic | Readiness is process-level only; investigate capture and forwarding separately |

For a systemd service, inspect systemctl status and systemctl cat for the actual unit and journalctl -u for logs. The legacy gateway unit redirects stdout to /var/log/aprs.log; inspect that file if journal output is incomplete. Keep access to logs restricted.

## Transmission

Only licensed operators using a verified RF chain should enable TX. Check audio device, PTT/VOX interface, levels, antenna isolation, local rules and beacon settings. Test with appropriate equipment and a rated dummy load before unattended operation. Software dispatch is not proof of RF emission. See [receiver protection](learn/rf-power-and-protection.md).

## Ask for help

If you're still stuck, open a [question or bug report](https://github.com/oorrwullie/sparky/issues/new/choose). Include the commit or release, OS, hardware, selected input/output path, what you expected, what happened and a small sanitized log excerpt. You don't have to diagnose the bug first.
