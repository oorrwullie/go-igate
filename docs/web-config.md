# Web configuration — implementation notes

> **Historical implementation note:** this document describes an earlier incremental UI stage, not the current feature set. For shipped behavior, setup, authentication, restart, and live status, use the [operator quick start](operator-quickstart.md).


The approved Stage 4 interface is a dark terminal-inspired Go web UI. This
first slice contains **only** the configuration-file storage primitive; it
does not expose an HTTP listener, render the interface, or restart the service.

- The live application loads `config.yml` from its working directory.
- `configeditor.Store` loads the original YAML bytes, preserving formatting,
  comments and unmodeled YAML fields. It uses a SHA-256 revision token to
  detect edits made since loading.
- Save runs the same semantic validation/defaulting routine used at startup,
  then atomically replaces the file in the same directory (mode 0600).
- File loading uses no-follow, nonblocking descriptor opening on Linux/macOS,
  so a swapped symlink or FIFO cannot bypass the regular-file requirement.
  On other operating systems this store refuses file access until a safe
  implementation exists.
- Once rename succeeds, the new revision is returned even if directory sync
  fails. In that case `errors.Is(err, configeditor.ErrDurability)` is true:
  the new contents are live, but crash durability is not confirmed. Do not
  blindly retry the save with the previous revision token.
- Save does not trigger a reload. A restart must be an explicit, separately
  authorized action after a successful save.
- The YAML file contains secrets. **Do not expose raw document bytes through
  an unauthenticated endpoint**. UI work must include authentication,
  same-origin/CSRF defenses, no-store responses, and redaction of credentials.
- The web UI must not accept arbitrary config paths or arbitrary service names.
  A restart must not grant unrestricted command execution or `systemctl`
  privileges to a browser-facing process.
- Multi-process writers require operator coordination; optimistic conflict
  detection alone does not provide an interprocess lock.
- When a systemd installation is used, the configured working directory and
  file ownership must grant appropriate write access for the explicit config
  editing deployment model. Do not silently run Sparky as root.

Next slices: a secure loopback-bound HTTP entry point and read-only page,
then editable forms/validation/save, followed by an explicit restart adapter
with separate permissions and confirmation.
