# Web configuration: first structured editor

> **Historical implementation note:** this document describes an earlier incremental UI stage, not the current feature set. For shipped behavior, setup, authentication, restart, and live status, use the [operator quick start](operator-quickstart.md).


This slice adds `/edit` to the standalone `sparky-config` interface from PR #46.
It is not started by the normal Sparky service.

Set an environment variable before starting the editor:

```sh
export SPARKY_CONFIG_PASSWORD='choose-a-unique-long-password'
go run ./cmd/sparky-config --config ./config.yml
```

The **entire interface** (overview, editor, and all other paths) requires a
login. Enter username `admin` and the configured password when your browser
prompts for HTTP Basic authentication. No page is served if the environment
variable is missing or fewer than 16 characters. Use a unique, randomly
created password, not a callsign or another reused credential.

This deliberately has no users table, sessions, or password database. By
default it binds to `127.0.0.1:8080` over HTTP. For another machine, you can
use an SSH port forward **or opt into direct HTTPS** on a LAN address:

```sh
export SPARKY_CONFIG_PASSWORD='a-long-unique-random-password'
go run ./cmd/sparky-config --listen 0.0.0.0:8443 \
  --config ./config.yml \
  --tls-cert /path/to/fullchain.pem \
  --tls-key /path/to/privatekey.pem
```

Visit `https://<station-hostname>:8443/` using a hostname covered by the TLS
certificate and trusted by the browser. You can bind to a particular LAN IP
instead of every interface; a firewall should restrict access to trusted
clients. Non-loopback listeners require TLS by default; the separate Routy LAN proxy
mode below is an explicit exception.
The UI requires login regardless of binding. HTTP Basic sends credentials
with requests, so it is only suitable here over HTTPS or local HTTP; do not
ignore certificate warnings. Plaintext proxying is supported only as the
explicitly restricted Routy LAN deployment described below.
## Routy on the same LAN

Routy can live on another machine. It terminates public HTTPS using its normal
Let's Encrypt configuration and proxies to Sparky's LAN address. Sparky
requires an explicit opt-in because this backend hop uses **unencrypted HTTP**:

```sh
export SPARKY_CONFIG_PASSWORD='a-long-unique-random-password'
go run ./cmd/sparky-config \
  --listen 192.168.1.42:8080 \
  --public-origin https://sparky.example.org \
  --trusted-proxy-ip 192.168.1.10 \
  --allow-lan-http
```

Replace the IP addresses with Sparky's and Routy's actual addresses.
Configure Routy's upstream to `http://192.168.1.42:8080`, preserve the
original browser `Host` header (`sparky.example.org`), and serve the
public hostname over trusted HTTPS. Sparky verifies the configured hostname,
the TCP source IP of Routy, the required login, and the exact HTTPS Origin
on form submissions. It does **not** trust `X-Forwarded-For` or
`X-Forwarded-Proto` as authorization evidence.

**Required network boundary:** firewall Sparky's port 8080 so only Routy's
LAN IP can reach it. Prefer a dedicated management VLAN or TLS on the
backend link if the LAN is not trusted. HTTP Basic credentials are sent on
every request, including the unencrypted LAN hop. For a fully encrypted
backend, use Sparky's built-in `--tls-cert`/`--tls-key` mode and configure
Routy to validate that upstream TLS certificate. Never expose the LAN HTTP
port directly to the public Internet.

Omit `--allow-lan-http` for the safe default: localhost-only HTTP, or
explicit HTTPS for non-loopback network access.

Exposing an administrative endpoint directly on the public Internet is not
recommended: this version does not provide a hardened login workflow,
account lockouts, or rate limiting. Browsers may cache Basic credentials;
close the private browser session to clear them rather than relying on a
Logout button.

The form currently edits station callsign and selected receive/forward/transmit
enable toggles. All other settings, including credentials, remain in the YAML
document and are not sent to the browser. The YAML document is the source of
truth. Comment-preserving YAML-node editing may normalize other whitespace and
formatting; do not promise byte-for-byte preservation. Save validates against
Sparky's startup parser, rejects a stale revision, and **does not restart**
or apply changes to the running application.

The editor uses an unpredictable CSRF token and requires a same-origin POST.
Browser response caching and framing are disabled. Both the overview and editor
require authentication; neither page exposes stored configuration credentials.
Use a distinct secret: this is a lightweight administrative interface, not a
hardened Internet-facing login service.

**The service restart control remains intentionally disabled.** It will be
implemented as a separately reviewed, narrowly privileged operation.
