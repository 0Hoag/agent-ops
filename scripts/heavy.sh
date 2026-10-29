#!/usr/bin/env bash
# Runs a heavy command (browser, docker compose, full build) one at a time across all workers, at low CPU priority.
#
#   scripts/heavy.sh <command> [args...]
#   scripts/heavy.sh pnpm exec playwright test
set -uo pipefail

lock=${TMPDIR:-/tmp}/agent-ops-heavy.lock
until mkdir "$lock" 2>/dev/null; do
  # A lock whose owner died (kill -9) is stale: take it over.
  if [ -f "$lock/pid" ] && ! kill -0 "$(cat "$lock/pid")" 2>/dev/null; then rm -rf "$lock"; continue; fi
  sleep 5
done
echo $$ > "$lock/pid"
trap 'rm -rf "$lock"' EXIT
nice -n 10 "$@"
