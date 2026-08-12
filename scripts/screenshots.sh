#!/usr/bin/env bash
#
# Regenerate the README screenshot.
#
#   scripts/screenshots.sh
#
# It is a real frame: the browser runs in a pty and the resulting screen is
# rendered to SVG. Only testdata/sample.mrc is used, so the image holds no
# third-party records and anyone can reproduce it.
#
# Needs nothing but Go and python3.
#
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

binary="$work/lunette"
go build -o "$binary" .

render() { python3 scripts/ansi2svg.py "$1" "$2" "$3"; }

capture() { python3 scripts/capture.py "$@"; }

sample=testdata/sample.mrc

# The default view: record list, annotated record, field cursor.
capture "$work/browse.txt" "$binary" "$sample" >/dev/null
render "$work/browse.txt" docs/screenshot-browse.svg "lunette browsing a MARC file"

