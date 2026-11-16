#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
VERIFY=$ROOT/scripts/verify/mcp.sh
tmp=$(mktemp -d "${TMPDIR:-/tmp}/mcp-cleanup-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

extract_cleanup() {
  sed -n '/^cleanup_module_cache()/,/^}/p' "$1"
}

run_fixture() {
  local script=$1 expected=$2 run=$3 status=0
  mkdir -p "$run/modcache/example.com/disposable@v1.0.0"
  printf 'package disposable\n' >"$run/modcache/example.com/disposable@v1.0.0/go.mod"
  chmod 555 "$run/modcache/example.com/disposable@v1.0.0"
  chmod 444 "$run/modcache/example.com/disposable@v1.0.0/go.mod"
  RUN="$run" bash -c "$(extract_cleanup "$script"); trap cleanup_module_cache EXIT; exit '$expected'" || status=$?
  [[ "$status" -eq "$expected" ]] || {
    echo "FAIL: cleanup exit $status, expected $expected" >&2
    return 1
  }
  [[ ! -e "$run/modcache" ]] || {
    echo "FAIL: owned module cache remains" >&2
    return 1
  }
}

run_fixture "$VERIFY" 0 "$tmp/green"

control="$tmp/old-cleanup.sh"
printf '%s\n' 'cleanup_module_cache() {' '  rm -rf "$RUN"' '}' >"$control"
if run_fixture "$control" 0 "$tmp/old"; then
  echo 'FAIL: old cleanup unexpectedly removed read-only fixture' >&2
  exit 1
fi
echo 'PASS: old cleanup fails the read-only module-cache regression'
chmod -R u+w "$tmp/old"

run_fixture "$VERIFY" 23 "$tmp/status"
echo 'PASS: owned read-only module cache is removed and original failure status is preserved'
