#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
FIXTURES=${SMOKE_CONTROL_FIXTURES:-"$(dirname "$ROOT")/w3w-smoke-controls-fixtures"}
SOURCE_SHA=$(git -C "$ROOT" rev-parse HEAD)
COMPOSE_SOURCE_SHA=$(git -C "$ROOT" rev-parse origin/feat/w3e-local-compose)
CLIENT="$FIXTURES/client"
PORT_FILE="$FIXTURES/ports"
COMPOSE="$FIXTURES/docker-compose.yml"
MUTATED_SMOKE="$FIXTURES/smoke-no-service-selection.sh"
CONTROL_LOG="$FIXTURES/ownership-control.log"
OWNED_PROJECT="agent-ops-smoke-control-$(id -u)-$$"
PREVIEW_PID=

mkdir -p "$FIXTURES"
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ -n "$PREVIEW_PID" ]]; then
    kill "$PREVIEW_PID" 2>/dev/null || true
    for _ in $(seq 1 30); do
      if ! lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then break; fi
      sleep 0.1
    done
  fi
  docker compose -p "$OWNED_PROJECT" -f "$COMPOSE" down --volumes --remove-orphans >/dev/null 2>&1 || true
  if [[ -n "${OWNED_PROJECT:-}" ]]; then
    docker ps -aq --filter "label=com.docker.compose.project=$OWNED_PROJECT" | xargs -r docker rm -f >/dev/null 2>&1 || true
    docker volume ls -q --filter "label=com.docker.compose.project=$OWNED_PROJECT" | xargs -r docker volume rm >/dev/null 2>&1 || true
  fi
  if [[ -n "${PREVIEW_PID:-}" ]] && lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then
    printf 'cleanup failed: owned preview listener remains on %s\n' "$CLIENT_PORT" >&2
    status=1
  fi
  if [[ -n "${COMPOSE:-}" && -f "$COMPOSE" ]]; then
    if docker ps -a --filter "label=com.docker.compose.project=$OWNED_PROJECT" --format '{{.ID}}' | grep -q . || docker volume ls --filter "label=com.docker.compose.project=$OWNED_PROJECT" --format '{{.Name}}' | grep -q .; then
      printf 'cleanup failed: owned Compose resources remain for %s\n' "$OWNED_PROJECT" >&2
      status=1
    fi
  fi
  if [[ $status -eq 0 ]]; then printf 'cleanup verified: no owned listeners, containers, or volumes\n'; fi
  exit "$status"
}
trap cleanup EXIT INT TERM

if [[ -e "$FIXTURES"/smoke-no-service-selection.sh ]]; then
  printf 'fixture path already contains a prior fixture: %s\n' "$FIXTURES" >&2
  exit 1
fi
git show "${COMPOSE_SOURCE_SHA}:docker-compose.yml" >"$COMPOSE"
python3 - "$COMPOSE" "$ROOT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
s = s.replace('context: ./api', f'context: {sys.argv[2]}/api')
s = s.replace('    ports:\n      - "${POSTGRES_PUBLISH-127.0.0.1}:${POSTGRES_HOST_PORT-5432}:5432"\n', '')
p.write_text(s)
PY
printf 'Compose fixture source SHA: %s\n' "$COMPOSE_SOURCE_SHA"
if [[ ! -d "$CLIENT/.git" ]]; then git clone https://github.com/0Hoag/agent-ops-client.git "$CLIENT"; fi
if [[ -n "$(git -C "$CLIENT" status --porcelain --untracked-files=all)" ]]; then
  printf 'owned client clone is not clean: %s\n' "$CLIENT" >&2
  exit 1
fi
git -C "$CLIENT" fetch origin main
git -C "$CLIENT" merge --ff-only origin/main

python3 - "$PORT_FILE" <<'PY'
import socket, sys
ports = []
for _ in range(2):
    sock = socket.socket()
    sock.bind(("127.0.0.1", 0))
    ports.append(sock.getsockname()[1])
    sock.close()
open(sys.argv[1], "w").write(" ".join(map(str, ports)) + "\n")
PY
read -r API_PORT CLIENT_PORT <"$PORT_FILE"
export CLIENT_DIR="$CLIENT" SMOKE_API_PORT="$API_PORT" SMOKE_CLIENT_PORT="$CLIENT_PORT"
export SMOKE_ARTIFACT_DIR="$FIXTURES/artifacts"
export SMOKE_CONTROL_COMPOSE="$COMPOSE"
printf 'Source SHA: %s\nClient port: %s\n' "$SOURCE_SHA" "$CLIENT_PORT"

docker compose -p "$OWNED_PROJECT" -f "$COMPOSE" up -d --build postgres api
for _ in $(seq 1 90); do
  if curl --fail --silent "http://127.0.0.1:$API_PORT/readyz" >/dev/null; then break; fi
  sleep 1
done
curl --fail --silent "http://127.0.0.1:$API_PORT/readyz" >/dev/null
services=$(docker compose -p "$OWNED_PROJECT" -f "$COMPOSE" ps --services --filter status=running | sort)
if [[ "$services" != $'api\npostgres' ]]; then
  printf 'unexpected Compose services: %s\n' "${services//$'\n'/, }" >&2
  exit 1
fi
compose_client=$(docker compose -p "$OWNED_PROJECT" -f "$COMPOSE" ps -q client)
if [[ -n "$compose_client" ]]; then
  printf 'unexpected Compose client container: %s\n' "$compose_client" >&2
  exit 1
fi
api_pid=$(docker compose -p "$OWNED_PROJECT" -f "$COMPOSE" top api | awk 'NR==2 {print $2}')
api_listener=$(lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print $2}' | sort -u)
if [[ -z "$api_listener" ]]; then printf 'expected API listener missing on %s\n' "$API_PORT" >&2; exit 1; fi
printf 'baseline ownership: services=%s api-host-listener-pid=%s (container-init=%s)\n' "${services//$'\n'/,}" "$api_listener" "$api_pid"

# The control exercises the production script with identical ports, Compose and client inputs.
cp "$ROOT/scripts/smoke.sh" "$MUTATED_SMOKE"
python3 - "$MUTATED_SMOKE" "$ROOT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); root = sys.argv[2]
s = p.read_text()
old = 'up -d --build postgres api'
assert s.count(old) == 1
s = s.replace(old, 'up -d --build', 1)
s = s.replace('ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)', f'ROOT="{root}"')
s = s.replace('"$ROOT/docker-compose.yml"', '"$SMOKE_CONTROL_COMPOSE"')
p.write_text(s)
PY
chmod +x "$MUTATED_SMOKE"
set +e
SMOKE_PROJECT="${OWNED_PROJECT}-control" bash "$MUTATED_SMOKE" >"$CONTROL_LOG" 2>&1
control_status=$?
set -e
if [[ $control_status -eq 0 ]] || ! grep -E 'unexpected Compose client|unexpected Compose services' "$CONTROL_LOG"; then
  printf 'service-selection mutation did not fail the ownership assertion (exit %s)\n' "$control_status" >&2
  exit 1
fi
printf 'ownership control exit: %s\n' "$control_status"
grep -E 'unexpected Compose client|unexpected Compose services' "$CONTROL_LOG"

# Independent real child lifecycle: wrapper exits; listener remains until actual child cleanup.
python3 - "$CLIENT_PORT" "$FIXTURES/preview-child.pid" <<'PY'
import http.server, os, socketserver, sys
class Server(socketserver.TCPServer): allow_reuse_address = True
with Server(("127.0.0.1", int(sys.argv[1])), http.server.SimpleHTTPRequestHandler) as server:
    with open(sys.argv[2], "w") as f: f.write(str(os.getpid()))
    server.serve_forever()
PY
  >"$FIXTURES/preview-child.log" 2>&1 &
PREVIEW_PID=$!
for _ in $(seq 1 50); do [[ -s "$FIXTURES/preview-child.pid" ]] && break; sleep 0.1; done
CHILD_PID=$(<"$FIXTURES/preview-child.pid")
child_listener=$(lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print $2}' | sort -u)
[[ "$child_listener" == "$CHILD_PID" ]] || { printf 'child listener ownership mismatch: expected %s got %s\n' "$CHILD_PID" "$child_listener" >&2; exit 1; }
kill "$PREVIEW_PID"
wait "$PREVIEW_PID" 2>/dev/null || true
PREVIEW_PID=
kill -0 "$CHILD_PID"
if ! lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print $2}' | grep -qx "$CHILD_PID"; then
  printf 'child listener did not survive wrapper exit (wrapper=%s child=%s)\n' "$!" "$CHILD_PID" >&2
  exit 1
fi
printf 'child lifecycle baseline: wrapper exited; child pid=%s still owns listener %s\n' "$CHILD_PID" "$CLIENT_PORT"
kill "$CHILD_PID"
for _ in $(seq 1 50); do if ! lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then break; fi; sleep 0.1; done
if lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then
  printf 'cleanup failed: preview listener remains on %s\n' "$CLIENT_PORT" >&2
  exit 1
fi
printf 'baseline cleanup assertion: listener removed after child pid %s terminated\n' "$CHILD_PID"

# Mutation bypasses the production descendant-kill loop. The unchanged assertion must reject it.
MUTATED_CLEANUP="$FIXTURES/smoke-no-descendant-cleanup.sh"
cp "$ROOT/scripts/smoke.sh" "$MUTATED_CLEANUP"
python3 - "$MUTATED_CLEANUP" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
old = 'for child in $preview_tree; do kill "$child" 2>/dev/null || true; done'
assert s.count(old) == 1
p.write_text(s.replace(old, ': # mutation: skip descendant cleanup', 1))
PY
chmod +x "$MUTATED_CLEANUP"
CHILD_LOG="$FIXTURES/mutation-child.log"
CHILD_PID_FILE="$FIXTURES/mutation-child.pid"
python3 - "$CLIENT_PORT" "$CHILD_PID_FILE" <<'PY'
import http.server, os, socketserver, sys
class Server(socketserver.TCPServer): allow_reuse_address = True
with Server(("127.0.0.1", int(sys.argv[1])), http.server.SimpleHTTPRequestHandler) as server:
    with open(sys.argv[2], "w") as f: f.write(str(os.getpid()))
    server.serve_forever()
PY
  >"$CHILD_LOG" 2>&1 &
PREVIEW_PID=$!
for _ in $(seq 1 50); do [[ -s "$CHILD_PID_FILE" ]] && break; sleep 0.1; done
CHILD_PID=$(<"$CHILD_PID_FILE")
[[ "$(lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print $2}' | sort -u)" == "$CHILD_PID" ]]
set +e
SMOKE_PROJECT="${OWNED_PROJECT}-leak-control" bash "$MUTATED_CLEANUP" >"$FIXTURES/cleanup-control.log" 2>&1
control_status=$?
set -e
if [[ $control_status -eq 0 ]] || ! grep -F 'cleanup failed: preview listener remains' "$FIXTURES/cleanup-control.log"; then
  kill "$CHILD_PID" 2>/dev/null || true
  printf 'descendant-cleanup mutation did not fail unchanged listener assertion (exit %s)\n' "$control_status" >&2
  exit 1
fi
printf 'cleanup control exit: %s\n' "$control_status"
grep -F 'cleanup failed: preview listener remains' "$FIXTURES/cleanup-control.log"
kill "$CHILD_PID" 2>/dev/null || true
for _ in $(seq 1 50); do if ! lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then break; fi; sleep 0.1; done
[[ -z "$(lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null || true)" ]]
PREVIEW_PID=
printf 'verification complete: source=%s fixture-compose=%s ports=%s,%s\n' "$SOURCE_SHA" "$COMPOSE_SOURCE_SHA" "$API_PORT" "$CLIENT_PORT"
