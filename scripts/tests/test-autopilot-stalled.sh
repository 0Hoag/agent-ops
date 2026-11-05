#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"
cp "$root/scripts/autopilot.sh" "$tmp/autopilot.sh"
grep -q 'stalled: no worker, GitHub unchanged, no pending work' "$tmp/autopilot.sh" || { echo 'ASSERT stalled outcome missing'; exit 1; }
grep -q 'refusing to treat API error as idle' "$tmp/autopilot.sh" || { echo 'ASSERT API error explicit missing'; exit 1; }
grep -q 'title,body,updatedAt' "$tmp/autopilot.sh" || { echo 'ASSERT issue content fingerprint missing'; exit 1; }
echo 'PASS autopilot stalled, API error and issue-content guards'
