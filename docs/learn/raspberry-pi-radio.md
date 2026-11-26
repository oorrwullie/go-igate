# A little computer makes a pretty good radio station

One of my favorite things about small Linux boards is how much you can do with them. You don't need a full-size desktop running all day just to collect packets.

A Raspberry Pi Zero 2 W is an interesting low-power candidate for a simple receive-only station. A Pi 4 or 5, or another Linux SBC, gives you more headroom for multiple services. Performance depends on the decoder, hardware, storage and workload; don't assume every board can do every job.

## Start with the simplest setup

Install Raspberry Pi OS Lite or another supported Linux distribution for your board, configure SSH with key authentication and keep it updated. Use a reliable power supply, enough storage, and a USB adapter/hub appropriate for your SDR and board. Some small boards only have one USB data port.

Follow the [first APRS station](first-aprs-station.md) or [ADS-B receiver](first-adsb-receiver.md) experiment first. Run it interactively and confirm you're actually receiving before setting up a background service.

Then consider the [Sparky operator guide](../operator-quickstart.md) for a managed station. Use a non-root service account with narrowly scoped hardware access, avoid exposing admin interfaces, and check logs and shutdown behavior. Keep a backup of your configuration and don't publish credentials.

## What to measure

Look at CPU load, memory, decoder output, dropped data and temperature during a realistic workload. You want the station to keep receiving even when you're not watching it. A tiny board that's reliable is far more useful than a fast one that keeps falling over.

A self-contained Sparky appliance with integrated transmit hardware is a future project. This tutorial doesn't require that hardware.
