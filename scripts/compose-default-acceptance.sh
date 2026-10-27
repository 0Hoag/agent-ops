#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
project="compose-default-acceptance-${USER:-user}-$$"
api_port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
db_port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
cleanup() {
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export SMOKE_API_PORT=$api_port POSTGRES_HOST_PORT=$db_port
export COMPOSE_PROJECT_NAME=$project
compose_file=${COMPOSE_FILE:-$ROOT/docker-compose.yml}
compose() { docker compose -f "$compose_file" -p "$project" "$@"; }
assert_config() {
  local expected=$1 actual
  actual=$(compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["services"]["api"]["environment"]["SCM_REPOSITORIES_JSON"])')
  [[ "$actual" == "$expected" ]] || { printf 'SCM_REPOSITORIES_JSON mismatch: expected %s, got %s\n' "$expected" "$actual" >&2; return 1; }
  python3 -c 'import json,sys; value=json.loads(sys.argv[1]); assert isinstance(value,list)' "$actual"
}
unset SCM_REPOSITORIES_JSON
export JWT_SECRET=fixture-jwt REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901
if [[ ${EXPECT_OLD_DEFAULT_CONTROL:-0} == 1 ]]; then
  if assert_config '[]'; then
    printf 'FAIL: old Compose scalar unexpectedly passed the empty-array assertion\n' >&2
    exit 1
  else
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
    printf 'PASS: old Compose scalar failed the intended SCM_REPOSITORIES_JSON assertion\n'
    exit 0
  fi
fi
assert_config '[]'
printf 'PASS: unset default is JSON empty array []\n'
configured='[{"provider":"github","full_name":"fixture/repo"}]'
actual=$(SCM_REPOSITORIES_JSON="$configured" JWT_SECRET=fixture-jwt REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["services"]["api"]["environment"]["SCM_REPOSITORIES_JSON"])')
[[ "$actual" == "$configured" ]] || { printf 'explicit JSON changed: %s\n' "$actual" >&2; exit 1; }
printf 'PASS: explicit JSON preserved byte-for-byte\n'
compose up -d --build api
for attempt in {1..60}; do
  if curl --silent --fail "http://127.0.0.1:${api_port}/readyz" >/dev/null 2>&1; then
    printf 'PASS: disposable Compose API /readyz returned HTTP 200 (API port %s, Postgres port %s)\n' "$api_port" "$db_port"
    exit 0
  fi
  sleep 2
done
printf 'FAIL: disposable Compose API did not reach /readyz\n' >&2
exit 1
