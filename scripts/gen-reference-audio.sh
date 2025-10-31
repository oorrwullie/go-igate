#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/gen-reference-audio.sh [-m "APRS packet"] [-o /tmp/out.wav] [-d /path/to/direwolf]

Generate a reference APRS AFSK1200 audio sample with Direwolf's gen_packets
and validate it with multimon-ng.

Options:
  -m  Packet to encode (TNC2 monitor format). Default:
        N7RIX-10>APRS,WIDE1-1,WIDE2-1:=4010.30NS11137.60W#Powered by gophers!
  -o  Output WAV file (default: ./bin/reference-aprs.wav).
  -d  Direwolf source directory (default: ../direwolf relative to repo root).
  -h  Show this help message.
EOF
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
direwolf_root="${repo_root}/../direwolf"
message="N7RIX-10>APRS,WIDE1-1,WIDE2-1:=4010.30NS11137.60W#Powered by gophers!"
output="${repo_root}/bin/reference-aprs.wav"

while getopts ":m:o:d:h" opt; do
  case "$opt" in
    m) message="$OPTARG" ;;
    o) output="$(realpath "$OPTARG")" ;;
    d) direwolf_root="$(realpath "$OPTARG")" ;;
    h) usage; exit 0 ;;
    :) echo "Missing argument for -$OPTARG" >&2; usage; exit 1 ;;
    \?) echo "Unknown option -$OPTARG" >&2; usage; exit 1 ;;
  esac
done

gen_packets_bin="${direwolf_root}/build/src/gen_packets"
atest_bin="${direwolf_root}/build/src/atest"

if [[ ! -x "$gen_packets_bin" || ! -x "$atest_bin" ]]; then
  echo "[info] Building Direwolf helper utilities..."
  mkdir -p "${direwolf_root}/build"
  cmake -S "$direwolf_root" -B "${direwolf_root}/build"
  cmake --build "${direwolf_root}/build" --target gen_packets atest
fi

mkdir -p "$(dirname "$output")"
echo "$message" | "$gen_packets_bin" -B 1200 -o "$output" -
echo "[info] Generated $output"

if command -v multimon-ng >/dev/null 2>&1; then
  echo "[info] Verifying with multimon-ng..."
  multimon-ng -a AFSK1200 -A -t wav "$output"
else
  echo "[warn] multimon-ng not found; skipping decode check" >&2
fi

echo "[info] You can play the sample with: aplay \"$output\""
