# Publishing the learning library to GitHub Wiki

The source of truth is `docs/learn/` in the main repository. Changes are reviewed in regular pull requests. The wiki is a friendly doorway to those reviewed articles, not a second independently maintained copy.

## One-time setup

1. Open the repository's Settings, under General find Features, and enable Wikis if necessary. Keep editing restricted to collaborators.
2. Open the Wiki tab and select Create the first page. Name it Home.
3. Paste the following Markdown and save.

~~~markdown
# Welcome to the Sparky workbench!

Radio is one of those hobbies where the more you learn, the more interesting things you find to learn about. You don't need a callsign, a programming background, or a big budget to start exploring.

## Learn the basics

- [Getting your amateur radio license (US)](https://github.com/oorrwullie/sparky/blob/main/docs/learn/amateur-radio-license.md)
- [APRS](https://github.com/oorrwullie/sparky/blob/main/docs/learn/aprs.md)
- [iGates](https://github.com/oorrwullie/sparky/blob/main/docs/learn/igate.md)
- [Digipeaters](https://github.com/oorrwullie/sparky/blob/main/docs/learn/digipeaters.md)
- [Software-defined radio](https://github.com/oorrwullie/sparky/blob/main/docs/learn/sdr.md)
- [ADS-B](https://github.com/oorrwullie/sparky/blob/main/docs/learn/adsb.md)
- [MQTT](https://github.com/oorrwullie/sparky/blob/main/docs/learn/mqtt.md)

## Get your hands dirty

- [Your first APRS receiver](https://github.com/oorrwullie/sparky/blob/main/docs/learn/first-aprs-station.md)
- [Your first ADS-B receiver](https://github.com/oorrwullie/sparky/blob/main/docs/learn/first-adsb-receiver.md)
- [Antennas, gain and feedline](https://github.com/oorrwullie/sparky/blob/main/docs/learn/antennas-and-feedline.md)
- [RF power and receiver protection](https://github.com/oorrwullie/sparky/blob/main/docs/learn/rf-power-and-protection.md)
- [Raspberry Pi radio projects](https://github.com/oorrwullie/sparky/blob/main/docs/learn/raspberry-pi-radio.md)
- [Troubleshooting your station](https://github.com/oorrwullie/sparky/blob/main/docs/learn/station-troubleshooting.md)
- [Packet collisions](https://github.com/oorrwullie/sparky/blob/main/docs/learn/packet-collisions.md)
- [Build your own radio gateway](https://github.com/oorrwullie/sparky/blob/main/docs/learn/build-a-gateway.md)

## Running Sparky

[Operator documentation](https://github.com/oorrwullie/sparky/blob/main/docs/README.md) · [Quick start](https://github.com/oorrwullie/sparky/blob/main/docs/operator-quickstart.md)

These guides live in the main repository so they can be reviewed, corrected and improved through pull requests. Come explore with us!
~~~

Publish the home page after the learning-library PR is merged, so all links resolve.

## If we want full articles rendered in the wiki later

GitHub stores wiki pages in a separate Git repository. After creating Home in the web interface, clone it with:

~~~sh
git clone https://github.com/oorrwullie/sparky.wiki.git
~~~

Wiki commits don't go through the main repository PR process. If full articles are mirrored, generate them from approved source with wiki-relative links rewritten, review the changes in the wiki clone, and publish explicitly. Never maintain two independent article copies. For now, the linked home page keeps it simple.

See [GitHub's wiki editing guide](https://docs.github.com/en/communities/documenting-your-project-with-wikis/adding-or-editing-wiki-pages) for more.
