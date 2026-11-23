# Web configuration: first structured editor

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
go run ./cmd/sparky-config --listen 0.0.0.0:8443 \\
  --config ./config.yml \\
  --tls-cert /path/to/fullchain.pem \\
  --tls-key /path/to/privatekey.pem
```

Visit `https://<station-hostname>:8443/` using a hostname covered by the TLS
certificate and trusted by the browser. You can bind to a particular LAN IP
instead of every interface; a firewall should restrict access to trusted
clients. Non-loopback listeners are **refused** without both TLS files.
The UI requires login regardless of binding. HTTP Basic sends credentials
with requests, so it is only suitable here over HTTPS or local HTTP; do not
ignore certificate warnings or use a plaintext reverse proxy to expose it.
If a reverse proxy is used, it must provide HTTPS to clients, restrict access,
and forward to Sparky over a trusted loopback connection.

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
Browser response caching and framing are disabled. Authentication is required for both the read-only overview and editor;
neither page exposes stored configuration credentials. Use a distinct secret: this is a lightweight local admin interface,
not a hardened Internet-facing administration system.

**The service restart control remains intentionally disabled.** It will be
implemented as a separately reviewed, narrowly privileged operation.
