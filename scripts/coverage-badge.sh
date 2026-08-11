#!/usr/bin/env bash
#
# Generate a coverage badge SVG from a Go coverage profile.
#
# Usage:
#   scripts/coverage-badge.sh [coverage.out] [docs/coverage.svg]
#
# With no arguments it runs the test suite first, so it works as a one-shot
# "measure and draw" command as well as a step in CI.
#
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Arguments may be relative to the caller's directory.
abspath() { case "$1" in /*) printf '%s\n' "$1" ;; *) printf '%s\n' "$PWD/$1" ;; esac; }
profile="$(abspath "${1:-$repo_root/coverage.out}")"
output="$(abspath "${2:-$repo_root/docs/coverage.svg}")"

if [[ ! -f "$profile" ]]; then
  echo "==> no profile at $profile, running tests" >&2
  (cd "$repo_root" && go test -coverprofile="$profile" ./...) >&2
fi

# "total:  (statements)  74.8%" -> 74.8
percent="$(cd "$repo_root" && go tool cover -func="$profile" \
  | awk '/^total:/ {gsub(/%/, "", $NF); print $NF}')"

if [[ -z "$percent" ]]; then
  echo "error: could not read a total from $profile" >&2
  exit 1
fi

# Shields.io colour scale.
color="#e05d44" # red
awk_ge() { awk -v a="$percent" -v b="$1" 'BEGIN {exit !(a >= b)}'; }
if   awk_ge 90; then color="#4c1"     # bright green
elif awk_ge 80; then color="#97ca00"  # green
elif awk_ge 70; then color="#dfb317"  # yellow
elif awk_ge 60; then color="#fe7d37"  # orange
fi

label="coverage"
value="${percent}%"
# 7px per character approximates Verdana 11 well enough for a badge.
label_width=$(( ${#label} * 7 + 10 ))
value_width=$(( ${#value} * 7 + 10 ))
total_width=$(( label_width + value_width ))

mkdir -p "$(dirname "$output")"
cat > "$output" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
     width="$total_width" height="20" role="img" aria-label="$label: $value">
  <title>$label: $value</title>
  <linearGradient id="s" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r"><rect width="$total_width" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)">
    <rect width="$label_width" height="20" fill="#555"/>
    <rect x="$label_width" width="$value_width" height="20" fill="$color"/>
    <rect width="$total_width" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif"
     text-rendering="geometricPrecision" font-size="110">
    <text x="$(( label_width * 5 ))" y="150" fill="#010101" fill-opacity=".3"
          transform="scale(.1)" textLength="$(( (label_width - 10) * 10 ))">$label</text>
    <text x="$(( label_width * 5 ))" y="140" transform="scale(.1)"
          textLength="$(( (label_width - 10) * 10 ))">$label</text>
    <text x="$(( label_width * 10 + value_width * 5 ))" y="150" fill="#010101" fill-opacity=".3"
          transform="scale(.1)" textLength="$(( (value_width - 10) * 10 ))">$value</text>
    <text x="$(( label_width * 10 + value_width * 5 ))" y="140" transform="scale(.1)"
          textLength="$(( (value_width - 10) * 10 ))">$value</text>
  </g>
</svg>
EOF

echo "==> $output ($label $value)"
