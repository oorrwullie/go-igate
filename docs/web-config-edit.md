# Web configuration: first structured editor

This slice adds `/edit` to the standalone loopback-only `sparky-config`
preview introduced in PR #46. It is based on that PR and must merge **after**
#46. It is not started by the normal Sparky service.

Set an environment variable before starting the editor:

```sh
export SPARKY_CONFIG_PASSWORD='choose-a-unique-long-password'
go run ./cmd/sparky-config --config ./config.yml
```

Use the **Edit settings** link from the read-only overview and supply the
password in the browser's Basic authentication prompt. The configured password
must be at least 16 characters. Use a local browser or an SSH local port
forward; this HTTP interface must never be exposed directly to a network.

The form currently edits station callsign and selected receive/forward/transmit
enable toggles. All other settings, including credentials, remain in the YAML
document and are not sent to the browser. The YAML document is the source of
truth. Comment-preserving YAML-node editing may normalize other whitespace and
formatting; do not promise byte-for-byte preservation. Save validates against
Sparky's startup parser, rejects a stale revision, and **does not restart**
or apply changes to the running application.

The editor uses an unpredictable CSRF token and requires a same-origin POST.
Browser response caching and framing are disabled. Authentication is needed
for both GET and POST on `/edit`. The view-only `/` page does not expose
credentials. Use a distinct secret: this is a lightweight local admin interface,
not a hardened Internet-facing administration system.

**The service restart control remains intentionally disabled.** It will be
implemented as a separately reviewed, narrowly privileged operation.
