#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
tmp=$(mktemp -d)
trap 'kill "${server_pid:-}" 2>/dev/null || true; wait "${server_pid:-}" 2>/dev/null || true; rm -rf "$tmp"' EXIT
cat >"$tmp/ready.py" <<'PY'
from http.server import BaseHTTPRequestHandler, HTTPServer
import sys
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200 if self.path == '/readyz' else 404); self.end_headers()
    def log_message(self, *_): pass
HTTPServer(('127.0.0.1', int(sys.argv[1])), Handler).serve_forever()
PY
port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
python3 "$tmp/ready.py" "$port" >"$tmp/server.log" 2>&1 & server_pid=$!
OPERATIONS_UPGRADE_TEST_ONLY=1 OPERATIONS_UPGRADE_TEST_PID="$server_pid" OPERATIONS_UPGRADE_TEST_PORT="$port" OPERATIONS_UPGRADE_TEST_LOG="$tmp/server.log" bash "$root/scripts/verify/operations.sh"
printf 'PASS: production readiness seam accepts the owned ready child\n'
sh -c 'exit 0' & exited_pid=$!; wait "$exited_pid" || true
set +e
OPERATIONS_UPGRADE_TEST_ONLY=1 OPERATIONS_UPGRADE_TEST_CASE=exited OPERATIONS_UPGRADE_TEST_PID="$exited_pid" OPERATIONS_UPGRADE_TEST_LOG="$tmp/server.log" bash "$root/scripts/verify/operations.sh" >"$tmp/exited.log" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || { cat "$tmp/exited.log" >&2; exit 1; }
grep -Fq 'exited before readiness' "$tmp/exited.log"
printf 'PASS: exited child fails at named readiness assertion\n'
mutant="$tmp/mutant.sh"
sed 's/"\$OPERATIONS_UPGRADE_TEST_PORT"/"$((OPERATIONS_UPGRADE_TEST_PORT + 1))"/' "$root/scripts/verify/operations.sh" >"$mutant"
set +e
OPERATIONS_UPGRADE_TEST_ONLY=1 OPERATIONS_UPGRADE_READY_ATTEMPTS=1 OPERATIONS_UPGRADE_TEST_PID="$server_pid" OPERATIONS_UPGRADE_TEST_PORT="$port" OPERATIONS_UPGRADE_TEST_LOG="$tmp/server.log" bash "$mutant" >"$tmp/mutant.log" 2>&1
status=$?
set -e
[[ $status -ne 0 ]] || { cat "$tmp/mutant.log" >&2; exit 1; }
grep -Fq 'readiness timed out' "$tmp/mutant.log"
printf 'PASS: identical-input wrong-port control fails at readiness assertion\n'
kill "$server_pid"; wait "$server_pid" 2>/dev/null || true
