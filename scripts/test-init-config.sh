#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
dest="$tmp/bin"

make -s -C "$repo_root" init-config GOBIN="$dest"
test -f "$dest/config.yml"
cmp "$repo_root/config.yml.example" "$dest/config.yml"

# Sensitive station credentials should not be readable by other users.
permissions="$(stat -c '%a' "$dest/config.yml")"
test "$permissions" = "600" || { echo "new config permissions $permissions, expected 600" >&2; exit 1; }

grep -q '^enable-digipeater: false$' "$dest/config.yml"
awk '/^igate:/{flag=1;next} flag && /^  enabled:/{exit ($2=="false")?0:1}' "$dest/config.yml"
awk '/^transmitter:/{flag=1;next} flag && /^  enabled:/{exit ($2=="false")?0:1}' "$dest/config.yml"

# Refuse a second initialization and preserve the initial file byte-for-byte.
printf '\n# operator configuration\n' >> "$dest/config.yml"
cp "$dest/config.yml" "$tmp/preserved"
if make -s -C "$repo_root" init-config GOBIN="$dest" > "$tmp/output" 2>&1; then
  echo "init-config overwrote an existing config" >&2
  exit 1
fi
cmp "$tmp/preserved" "$dest/config.yml"

# Refuse an existing symlink, even if it points to a writable target.
mkdir "$tmp/symlink-bin"
printf 'SENTINEL\n' > "$tmp/target"
ln -s "$tmp/target" "$tmp/symlink-bin/config.yml"
if make -s -C "$repo_root" init-config GOBIN="$tmp/symlink-bin" > "$tmp/output" 2>&1; then
  echo "init-config wrote through a symlink" >&2
  exit 1
fi
test "$(cat "$tmp/target")" = SENTINEL

# An existing FIFO must not be opened, written to, or mistaken for success.
mkdir "$tmp/fifo-bin"
mkfifo "$tmp/fifo-bin/config.yml"
if timeout 5 make -s -C "$repo_root" init-config GOBIN="$tmp/fifo-bin" > "$tmp/output" 2>&1; then
  echo "init-config accepted an existing FIFO" >&2
  exit 1
fi
test -p "$tmp/fifo-bin/config.yml"

# A failed copy must never publish a partially copied config. The temp file
# is cleaned up even if the source cannot be read.
mkdir "$tmp/no-source"
if (cd "$tmp/no-source" && make -s -f "$repo_root/Makefile" init-config GOBIN="$tmp/no-source/bin" > "$tmp/output" 2>&1); then
  echo "init-config reported success without source template" >&2
  exit 1
fi
test ! -e "$tmp/no-source/bin/config.yml"

echo "first-run config creation, permissions and no-clobber tests passed"
 "$dest/config.yml"
awk '/^igate:/{flag=1;next} flag && /^  enabled:/{exit ($2=="false")?0:1}' "$dest/config.yml"
awk '/^transmitter:/{flag=1;next} flag && /^  enabled:/{exit ($2=="false")?0:1}' "$dest/config.yml"

# Refuse a second initialization and preserve the initial file byte-for-byte.
printf '\n# operator configuration\n' >> "$dest/config.yml"
cp "$dest/config.yml" "$tmp/preserved"
if make -s -C "$repo_root" init-config GOBIN="$dest" > "$tmp/output" 2>&1; then
  echo "init-config overwrote an existing config" >&2
  exit 1
fi
cmp "$tmp/preserved" "$dest/config.yml"

# Refuse an existing symlink, even if it points to a writable target.
mkdir "$tmp/symlink-bin"
printf 'SENTINEL\n' > "$tmp/target"
ln -s "$tmp/target" "$tmp/symlink-bin/config.yml"
if make -s -C "$repo_root" init-config GOBIN="$tmp/symlink-bin" > "$tmp/output" 2>&1; then
  echo "init-config wrote through a symlink" >&2
  exit 1
fi
test "$(cat "$tmp/target")" = SENTINEL

echo "first-run config creation, permissions and no-clobber tests passed"
