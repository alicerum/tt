#!/bin/sh
# Builds tt-cal (D40) into ~/.local/bin, or into the directory given as $1.
# The usage description is embedded in the binary: without it macOS (14+)
# never asks for calendar access.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out=${1:-$HOME/.local/bin}
mkdir -p "$out"
swiftc -O "$here/main.swift" -o "$out/tt-cal" \
	-Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker "$here/Info.plist"
echo "built $out/tt-cal · next: tt-cal calendars (the first run asks for calendar access)"
