#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Rendering must set the *actual* invoking user, not root or www-data.
sed -e "s|{{WORKING_DIRECTORY}}|$PWD/bin/|g" \
    -e "s|{{EXEC_START}}|$PWD/bin/sparky|g" \
    -e "s|{{SERVICE_USER}}|$(id -un)|g" \
    sparky.service.template > "$tmp/sparky.service"
if grep -q '{{' "$tmp/sparky.service"; then
  echo "Unresolved gateway unit placeholders" >&2
  exit 1
fi
grep -Fx "User=$(id -un)" "$tmp/sparky.service"
grep -Fx "WorkingDirectory=$PWD/bin/" "$tmp/sparky.service"
grep -Fx "ExecStart=$PWD/bin/sparky" "$tmp/sparky.service"
grep -Fx "StandardOutput=journal" "$tmp/sparky.service"
grep -Fx "NoNewPrivileges=true" "$tmp/sparky.service"
grep -Fx "ProtectSystem=strict" "$tmp/sparky.service"
grep -Fx "ReadWritePaths=$PWD/bin/" "$tmp/sparky.service"
if grep -Eq '^Group=(root|www-data)$|^StandardOutput=append:' "$tmp/sparky.service"; then
  echo "Legacy privileged group or log-append configuration remains" >&2
  exit 1
fi

# Inspect the *expanded* recipe; it must check real shell uid and prevent
# overwriting both /etc units and units elsewhere in systemd search paths.
make -n install-service > "$tmp/install-recipe"
grep -F '"$(id -u)" -ne 0' "$tmp/install-recipe"
grep -F 'sudo systemctl cat sparky.service' "$tmp/install-recipe"
grep -F 'go run ./cmd/sparky-device-check --config' "$tmp/install-recipe"
# Do not touch the service if the preflight detects an unavailable radio.
# The backend-specific unit tests provide fake sysfs/USB fixtures.
go test ./cmd/sparky-device-check -count=1

grep -F "set -C; umask 022; cat > /etc/systemd/system/sparky.service" "$tmp/install-recipe"

# A deliberately mismatched selected user must fail before invoking sudo.
if make -s install-service GATEWAY_SERVICE_USER=not-the-caller > "$tmp/rejected" 2>&1; then
  echo "Installer accepted a substituted service identity" >&2
  exit 1
fi
grep -q 'GATEWAY_SERVICE_USER must match' "$tmp/rejected"

if command -v systemd-analyze >/dev/null 2>&1; then
  # CI builds bin/sparky before this test; avoid unit runtime startup.
  systemd-analyze verify "$tmp/sparky.service"
fi

echo "gateway systemd non-root render and install preflight checks passed"
