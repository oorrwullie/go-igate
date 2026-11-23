# Web configuration: first structured editor

This slice adds `/edit` to the standalone loopback-only `sparky-config`
preview introduced in PR #46. It is based on that PR and must merge **after**
#46. It is not started by the normal Sparky service.

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

This deliberately has no users table, sessions, or password database. Basic
authentication credentials travel unencrypted over HTTP, so use a browser on
the same machine or an **encrypted SSH local port forward** from another
machine. Never expose the listener through a public proxy or directly bind it
to a LAN address. Browsers may cache Basic credentials; close the private
browser session to clear them rather than expecting a Logout button.

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
