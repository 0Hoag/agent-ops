#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
FIXTURES=${SMOKE_CONTROL_FIXTURES:-"$(dirname "$ROOT")/w3w-smoke-controls-fixtures"}
SOURCE_SHA=$(git -C "$ROOT" rev-parse HEAD)
COMPOSE_SOURCE_SHA=$(git -C "$ROOT" rev-parse origin/main)
CLIENT="$FIXTURES/client"
COMPOSE="$FIXTURES/docker-compose.yml"
OWNED_PROJECT="agent-ops-smoke-control-$(id -u)-$$"
CLIENT_PORT=
API_PORT=
LIFECYCLE_PORT=
PREVIEW_PID=
CHILD_PID=

mkdir -p "$FIXTURES"
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  for pid in "${CHILD_PID:-}" "${PREVIEW_PID:-}"; do
    [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
  done
  for port in "${CLIENT_PORT:-}" "${API_PORT:-}" "${LIFECYCLE_PORT:-}"; do
    [[ -n "$port" ]] || continue
    for _ in $(seq 1 50); do
      if ! lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | grep -q .; then break; fi
      sleep 0.1
    done
    if [[ "$port" == "$LIFECYCLE_PORT" ]] && lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | grep -q .; then
      printf 'cleanup failed: owned fixture listener remains on %s\n' "$port" >&2
      status=1
    fi
  done
  docker compose -p "$OWNED_PROJECT" -f "$COMPOSE" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker ps -aq --filter "label=com.docker.compose.project=$OWNED_PROJECT" | xargs -r docker rm -f >/dev/null 2>&1 || true
  docker volume ls -q --filter "label=com.docker.compose.project=$OWNED_PROJECT" | xargs -r docker volume rm >/dev/null 2>&1 || true
  if docker ps -a --filter "label=com.docker.compose.project=$OWNED_PROJECT" --format '{{.ID}}' | grep -q . || docker volume ls --filter "label=com.docker.compose.project=$OWNED_PROJECT" --format '{{.Name}}' | grep -q .; then
    printf 'cleanup failed: owned Compose resources remain\n' >&2
    status=1
  fi
  if [[ $status -eq 0 ]]; then printf 'cleanup verified: no owned listeners, containers, or volumes\n'; fi
  exit "$status"
}
trap cleanup EXIT INT TERM

if [[ -e "$COMPOSE" || -e "$CLIENT" || -e "$FIXTURES/source" ]]; then
  printf 'fixture path already contains a prior fixture: %s\n' "$FIXTURES" >&2
  exit 1
fi
git show "${COMPOSE_SOURCE_SHA}:docker-compose.yml" >"$COMPOSE"
python3 - "$COMPOSE" "$ROOT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
s = s.replace('context: ./api', f'context: {sys.argv[2]}/api')
s = s.replace('dockerfile: ${PWD}/deploy/local/Dockerfile', f'dockerfile: {sys.argv[2]}/deploy/local/Dockerfile')
s = s.replace('context: .\n      dockerfile:', f'context: {sys.argv[2]}\n      dockerfile:')
s = s.replace('    ports:\n      - "${POSTGRES_PUBLISH-127.0.0.1}:${POSTGRES_HOST_PORT-5432}:5432"\n', '')
p.write_text(s)
PY
printf 'Deployment Compose source SHA: %s\n' "$COMPOSE_SOURCE_SHA"
git clone https://github.com/0Hoag/agent-ops-client.git "$CLIENT"
git -C "$CLIENT" fetch origin main
git -C "$CLIENT" merge --ff-only origin/main

python3 - "$FIXTURES/ports" <<'PY'
import socket, sys
ports = []
for _ in range(2):
    sock = socket.socket(); sock.bind(("127.0.0.1", 0)); ports.append(sock.getsockname()[1]); sock.close()
open(sys.argv[1], "w").write(" ".join(map(str, ports)) + "\n")
PY
read -r API_PORT CLIENT_PORT <"$FIXTURES/ports"
python3 - "$FIXTURES/lifecycle-port" <<'PY'
import socket, sys
s = socket.socket(); s.bind(("127.0.0.1", 0)); open(sys.argv[1], "w").write(str(s.getsockname()[1]) + "\n"); s.close()
PY
LIFECYCLE_PORT=$(<"$FIXTURES/lifecycle-port")
export CLIENT_DIR="$CLIENT" SMOKE_API_PORT="$API_PORT" SMOKE_CLIENT_PORT="$CLIENT_PORT"
export SMOKE_ARTIFACT_DIR="$FIXTURES/artifacts" SMOKE_CONTROL_COMPOSE="$COMPOSE"
mkdir -p "$SMOKE_ARTIFACT_DIR"
printf 'Source SHA: %s\nAPI port: %s\nClient port: %s\n' "$SOURCE_SHA" "$API_PORT" "$CLIENT_PORT"

# Keep the full production browser smoke on its standard Compose input and isolate it from fixture Compose.
  cp "$ROOT/docker-compose.yml" "$FIXTURES/production-compose.yml"
python3 - "$FIXTURES/production-compose.yml" "$ROOT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
s = s.replace('context: ./api', f'context: {sys.argv[2]}/api')
p.write_text(s)
PY
# The production script references the repository Compose path; run from an owned temporary repo copy.
cp -R "$ROOT" "$FIXTURES/source"
cp "$FIXTURES/production-compose.yml" "$FIXTURES/source/docker-compose.yml"
ROOT="$FIXTURES/source" bash "$FIXTURES/source/scripts/smoke.sh" 2>&1 | tee "$FIXTURES/baseline.log"
BASELINE_PROJECT=$(sed -n 's/.*cleanup verified: no owned containers or volumes (\(.*\))/\1/p' "$FIXTURES/baseline.log" | tail -1)
[[ -n "$BASELINE_PROJECT" ]]
if docker ps -a --filter "label=com.docker.compose.project=$BASELINE_PROJECT" --format '{{.ID}}' | grep -q . || docker volume ls --filter "label=com.docker.compose.project=$BASELINE_PROJECT" --format '{{.Name}}' | grep -q .; then
  printf 'baseline left owned resources for %s\n' "$BASELINE_PROJECT" >&2; exit 1
fi
printf 'baseline production smoke: PASS (%s)\n' "$BASELINE_PROJECT"

# Add an actual client service to the deployment Compose, then use exactly the same production inputs.
python3 - "$COMPOSE" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
assert '\n  client:\n' in s
p.write_text(s)
PY
cp "$ROOT/scripts/smoke.sh" "$FIXTURES/source/scripts/smoke.sh"
python3 - "$FIXTURES/source/scripts/smoke.sh" "$ROOT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
s = s.replace('ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)', f'ROOT="{sys.argv[1].rsplit("/scripts/smoke.sh", 1)[0]}"')
s = s.replace('up -d --build postgres api', 'up -d --build')
p.write_text(s)
PY
set +e
SMOKE_PROJECT="${OWNED_PROJECT}-ownership-control" bash "$FIXTURES/source/scripts/smoke.sh" >"$FIXTURES/ownership-control.log" 2>&1
control_status=$?
set -e
if [[ $control_status -eq 0 ]] || ! grep -E 'unexpected Compose client/container/listener|unexpected Compose services' "$FIXTURES/ownership-control.log"; then
  printf 'service-selection mutation did not fail ownership assertion (exit %s)\n' "$control_status" >&2; exit 1
fi
printf 'ownership control exit: %s\n' "$control_status"
grep -E 'unexpected Compose client/container/listener|unexpected Compose services' "$FIXTURES/ownership-control.log"

# Isolate lifecycle probes in their own disposable copy of production smoke.sh.
cp "$ROOT/scripts/smoke.sh" "$FIXTURES/lifecycle-smoke.sh"
python3 - "$FIXTURES/lifecycle-smoke.sh" "$FIXTURES" <<'PY'
import pathlib, sys
p=pathlib.Path(sys.argv[1]); s=p.read_text()
s=s.replace('ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)', f'ROOT="{sys.argv[2]}"')
s=s.replace('trap cleanup EXIT', 'trap cleanup EXIT\nexit 0 # lifecycle probe exits with its preview descendant active')
p.write_text(s)
PY
cat >"$FIXTURES/lifecycle-preview.sh" <<'SH'
#!/usr/bin/env bash
python3 - "$SMOKE_CLIENT_PORT" "$SMOKE_ARTIFACT_DIR/descendant.pid" <<'PY' &
import http.server, os, pathlib, socketserver, sys
class Server(socketserver.TCPServer): allow_reuse_address = True
with Server(("127.0.0.1", int(sys.argv[1])), http.server.SimpleHTTPRequestHandler) as server:
    pathlib.Path(sys.argv[2]).write_text(str(os.getpid()))
    server.serve_forever()
PY
wait
SH
chmod +x "$FIXTURES/lifecycle-preview.sh"
python3 - "$FIXTURES/lifecycle-smoke.sh" "$FIXTURES/lifecycle-preview.sh" <<'PY'
import pathlib, sys
p=pathlib.Path(sys.argv[1]); s=p.read_text()
s=s.replace('pnpm --dir "$CLIENT" install --frozen-lockfile', ':')
s=s.replace('(cd "$CLIENT" && APP_API_URL="${SMOKE_BROWSER_API_URL:-$API_URL}" pnpm build)', ':')
s=s.replace('(cd "$CLIENT" && APP_API_URL="$API_URL" exec pnpm preview --host 127.0.0.1 --port "$CLIENT_PORT")', f'"{sys.argv[2]}"')
p.write_text(s)
PY
run_lifecycle_probe() {
  local label=$1 script=$2 out="$FIXTURES/$1.log" wrapper child listeners
  mkdir -p "$FIXTURES/artifacts"
  rm -f "$FIXTURES/artifacts/descendant.pid"
  SMOKE_CLIENT_PORT="$LIFECYCLE_PORT" SMOKE_API_PORT="$API_PORT" SMOKE_ARTIFACT_DIR="$FIXTURES/artifacts" SMOKE_PROJECT="${OWNED_PROJECT}-$label" bash "$script" >"$out" 2>&1 &
  PREVIEW_PID=$!
  for _ in $(seq 1 100); do
    [[ -s "$FIXTURES/artifacts/descendant.pid" ]] && break
    sleep 0.1
  done
  [[ -s "$FIXTURES/artifacts/descendant.pid" ]]
  CHILD_PID=$(<"$FIXTURES/artifacts/descendant.pid")
  wrapper=$(pgrep -P "$PREVIEW_PID" | head -1)
  [[ -n "$wrapper" && "$wrapper" != "$CHILD_PID" ]]
  listeners=$(lsof -nP -iTCP:"$LIFECYCLE_PORT" -sTCP:LISTEN 2>/dev/null | awk 'NR>1 {print $2}' | sort -u)
  [[ "$listeners" == "$CHILD_PID" ]]
  printf '%s observed wrapper=%s descendant=%s listener_pid=%s port=%s\n' "$label" "$wrapper" "$CHILD_PID" "$listeners" "$LIFECYCLE_PORT"
  wait "$PREVIEW_PID"; PREVIEW_PID=
  if [[ "$label" == baseline ]]; then
    if kill -0 "$CHILD_PID" 2>/dev/null || lsof -nP -iTCP:"$LIFECYCLE_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then
      printf 'baseline production cleanup left descendant/listener\n' >&2; return 1
    fi
    printf 'baseline production cleanup: PASS\n'
  else
    if ! grep -F 'cleanup failed: preview listener remains' "$out" || ! kill -0 "$CHILD_PID" 2>/dev/null; then
      printf 'descendant-cleanup mutation failed for the wrong reason\n' >&2; return 1
    fi
    printf 'cleanup control: expected listener assertion observed; descendant=%s survived wrapper cleanup\n' "$CHILD_PID"
    kill "$CHILD_PID" 2>/dev/null || true
    CHILD_PID=
    for _ in $(seq 1 50); do if ! lsof -nP -iTCP:"$LIFECYCLE_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then break; fi; sleep 0.1; done
  fi
}
run_lifecycle_probe baseline "$FIXTURES/lifecycle-smoke.sh"
cp "$FIXTURES/lifecycle-smoke.sh" "$FIXTURES/mutated-smoke.sh"
python3 - "$FIXTURES/mutated-smoke.sh" <<'PY'
import pathlib, sys
p=pathlib.Path(sys.argv[1]); s=p.read_text(); old='for child in $preview_tree; do kill -TERM "$child" 2>/dev/null || true; done'
assert s.count(old)==1
p.write_text(s.replace(old, ': # mutation bypasses production descendant termination', 1))
PY
run_lifecycle_probe mutation "$FIXTURES/mutated-smoke.sh"
# The restored source must pass the exact same lifecycle inputs.
run_lifecycle_probe restored "$FIXTURES/lifecycle-smoke.sh"
PREVIEW_PID= CHILD_PID=
printf 'verification complete: source=%s compose=%s ports=%s,%s\n' "$SOURCE_SHA" "$COMPOSE_SOURCE_SHA" "$API_PORT" "$CLIENT_PORT"
