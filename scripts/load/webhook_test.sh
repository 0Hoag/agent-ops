#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd); tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
printf '%s\n' '{"metrics":{"iterations":{"count":10},"iterations_completed":{"count":10},"dropped_iterations":{"count":0},"http_req_failed":{"rate":0}}}' >"$tmp/summary.json"
bash "$ROOT/scripts/load/reconcile-report.sh" "$tmp/summary.json" 10 10
if bash "$ROOT/scripts/load/reconcile-report.sh" "$tmp/summary.json" 9 10 >"$tmp/out" 2>&1; then exit 1; fi
grep -q 'persisted delivery count mismatch' "$tmp/out"
echo 'PASS: reconciliation rejects deliberately mismatched persisted count'
