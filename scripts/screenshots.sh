#!/usr/bin/env bash
#
# Regenerate the README screenshots.
#
#   scripts/screenshots.sh
#
# Each one is a real frame: the browser runs in a pty, keys are sent to it, and
# the resulting screen is rendered to SVG. Only testdata/sample.mrc is used, so
# the images hold no third-party records and anyone can reproduce them.
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

# 1. The default view: record list, annotated record, field cursor.
capture "$work/browse.txt" "$binary" "$sample" >/dev/null
render "$work/browse.txt" docs/screenshot-browse.svg "lunette browsing a MARC file"

# 2. A filter over the record body: list highlighting and the match counter.
#    "/" opens the prompt, "rec:" searches the record rather than the titles.
capture "$work/filter.txt" "$binary" "$sample" '/' 'rec:privacy' '\r' >/dev/null
render "$work/filter.txt" docs/screenshot-filter.svg "lunette filtering by subject"

# 3. Compact view with the record pane focused and the cursor on a field.
capture "$work/compact.txt" "$binary" "$sample" 'c' '\t' 'jjj' >/dev/null
render "$work/compact.txt" docs/screenshot-compact.svg "lunette compact view"
