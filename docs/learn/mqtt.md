# MQTT: how radios and computers talk to one another

MQTT sounds a lot more complicated than the basic idea really is. Instead of one program needing to know about every other program, it sends a message to a broker under a topic. Programs interested in that topic subscribe to it.

~~~text
OpenWebRX+ -> MQTT broker -> Sparky
                     |
                     +-----> another subscriber
~~~

The broker isn't the radio decoder. It moves messages. Topics are names, not RF frequencies. A subscriber can receive reports without being responsible for capturing the radio signal.

## Try it without touching a radio

Use a private local test broker and publish an ordinary text message on a topic like workbench/test. Subscribe to that topic in a second terminal using an MQTT client. Now you've seen the basic publish/subscribe idea.

Sparky supports an external MQTT broker or an embedded loopback broker for OpenWebRX+ reports. The embedded broker is for same-machine integration and allows anonymous local clients; don't expose it to the internet. Keep real RF input topics private so arbitrary clients cannot inject packets that Sparky treats as received radio traffic.

When forwarding ADS-B, Sparky uses MQTT QoS 0; completion doesn't mean the receiving application processed or displayed the report. That distinction matters when troubleshooting.

For configuration, follow the [OpenWebRX+ integration instructions](../../README.md#receiving-aprs-from-openwebrx). For the underlying protocol, see [MQTT.org](https://mqtt.org/).
