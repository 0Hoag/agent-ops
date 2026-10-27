#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-watch-acceptance-$(id -u)-$$"
API_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
PG_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
[[ "$API_PORT" != "$PG_PORT" ]] || { printf 'fixture: ports collided\n' >&2; exit 1; }
REPO=a9000000-0000-4000-8000-000000000001
TOKEN=$(python3 -c 'import secrets;print(secrets.token_hex(16))')
[[ $(printf %s "$TOKEN" | wc -c | tr -d ' ') == 32 ]] || { printf 'fixture: invalid token byte length\n' >&2; exit 1; }
export JWT_SECRET="synthetic-jwt-$TOKEN" REPOSITORY_ENCRYPTION_KEY="$TOKEN"
export POSTGRES_HOST_PORT="$PG_PORT" POSTGRES_PUBLISH=127.0.0.1
API="http://127.0.0.1:$API_PORT" DB="postgres://agent_ops:agent_ops_dev@127.0.0.1:$PG_PORT/agent_ops?sslmode=disable"
TMP=$(mktemp -d)
API_PID= WATCH_PID=
archive=${AGENT_WATCH_ARTIFACT_DIR:-}
log() { printf 'watch-acceptance: %s\n' "$*"; }
die() { log "FAIL: $*" >&2; exit 1; }
cleanup() {
  local status=$? p containers volumes
  trap - EXIT INT TERM
  for p in "$WATCH_PID" "$API_PID"; do [[ -n "$p" ]] && { kill "$p" 2>/dev/null || true; wait "$p" 2>/dev/null || true; } || true; done
  for p in "$WATCH_PID" "$API_PID"; do [[ -z "$p" ]] || ! kill -0 "$p" 2>/dev/null || { log "cleanup FAIL: PID $p remains"; status=1; }; done
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null 2>&1 || status=1
  for p in "$API_PORT" "$PG_PORT"; do lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1 && { log "cleanup FAIL: listener on $p"; status=1; } || true; done
  containers=$(docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' || true)
  volumes=$(docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' || true)
  [[ -z "$containers$volumes" ]] || { log "cleanup FAIL: owned resources remain"; status=1; }
  if [[ -n "$archive" ]]; then mkdir -p "$archive"; cp "$TMP"/*.log "$archive"/ 2>/dev/null || true; cp "$TMP"/*.status "$archive"/ 2>/dev/null || true; fi
  rm -rf "$TMP"
  (( status == 0 )) && log 'cleanup PASS: owned PIDs, listeners, containers, volumes absent'
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
BIN="$TMP/agentops"
(cd "$ROOT/api" && go build -o "$BIN" ./cmd/agentops && go build -o "$TMP/api" .)
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" up -d postgres >/dev/null
for _ in $(seq 1 60); do docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null || die 'Postgres readiness timeout'
export AGENT_RUN_TOKEN="$TOKEN" DATABASE_URL="$DB" HTTP_ADDR=":$API_PORT" SCM_REPOSITORIES_JSON='[]'
"$TMP/api" >"$TMP/api.log" 2>&1 &
API_PID=$!
ready=0
for _ in $(seq 1 60); do curl -fsS "$API/readyz" >/dev/null 2>&1 && { ready=1; break; }; sleep 1; done
(( ready )) || { cat "$TMP/api.log" >&2; die 'API readiness timeout'; }
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops -c "INSERT INTO repositories (id,provider,full_name,webhook_secret_env_ref) VALUES ('$REPO','github','acceptance/watch','ACCEPTANCE_SECRET')" >/dev/null
WATCH="$TMP/projects/p/fixture.jsonl"; CHECKPOINT="$TMP/checkpoint.json"
mkdir -p "$(dirname "$WATCH")"
line() { printf '{"type":"assistant","uuid":"%s","sessionId":"synthetic-session","timestamp":"2026-10-27T01:00:00Z","message":{"model":"claude-synthetic","usage":{"input_tokens":%s,"output_tokens":%s,"cache_read_input_tokens":%s,"cache_creation_input_tokens":%s}}}\n' "$1" "$2" "$3" "$4" "$5"; }
sql() { docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT count(*),coalesce(min(input_tokens),0),coalesce(max(input_tokens),0),coalesce(min(output_tokens),0),coalesce(max(output_tokens),0),coalesce(min(cached_input_tokens),0),coalesce(max(cached_input_tokens),0),coalesce(min(cache_write_tokens),0),coalesce(max(cache_write_tokens),0),coalesce(min(reasoning_tokens),0),coalesce(max(reasoning_tokens),0) FROM agent_runs WHERE repository_id='$REPO' AND harness='claude' AND session_id='synthetic-session'"; }
expect() { local expected=$1 label=$2 actual; for _ in $(seq 1 40); do actual=$(sql); [[ "$actual" == "$expected" ]] && { log "$label SQL=$actual"; return; }; kill -0 "$WATCH_PID" 2>/dev/null || { cat "$TMP/watch.log" >&2; die "$label watcher exited (status $(cat "$TMP/watch.status" 2>/dev/null || true)); SQL=$actual"; }; sleep .25; done; die "$label expected $expected got $(sql)"; }
alive() { kill -0 "$WATCH_PID" 2>/dev/null || die "$1: watcher not alive"; log "$1 watcher alive"; }
start_watch() {
  "$BIN" watch --root "$TMP/projects" --checkpoint "$CHECKPOINT" --endpoint "$API/api/v1/agent-runs" --repository-id "$REPO" --wave W9k --worker fixture --interval 1s >"$TMP/watch.log" 2>&1 & WATCH_PID=$!
}
stop_watch() { kill -TERM "$WATCH_PID"; wait "$WATCH_PID" || { cat "$TMP/watch.log" >&2; die 'watcher shutdown failed'; }; WATCH_PID=; }
line m1 10 2 4 3 > "$WATCH"
start_watch
expect '1|10|10|2|2|4|4|3|3|0|0' initial
line m2 15 3 5 4 >> "$WATCH"
expect '1|25|25|5|5|9|9|7|7|0|0' appended
printf '%s' '{"type":"assistant","uuid":"partial"' >> "$WATCH"
sleep 2
[[ "$(sql)" == '1|25|25|5|5|9|9|7|7|0|0' ]] || die 'partial-write assertion: SQL changed'
alive partial-write
printf '%s\n' ',"sessionId":"synthetic-session","message":{"model":"claude-synthetic","usage":{"input_tokens":7,"output_tokens":1,"cache_read_input_tokens":2,"cache_creation_input_tokens":1}}}' >> "$WATCH"
expect '1|32|32|6|6|11|11|8|8|0|0' completed
sleep 2
[[ "$(sql)" == '1|32|32|6|6|11|11|8|8|0|0' ]] || die 'no-op poll assertion: SQL changed'
alive no-op-poll
stop_watch
start_watch
expect '1|32|32|6|6|11|11|8|8|0|0' restart-no-op
sleep 2
[[ "$(sql)" == '1|32|32|6|6|11|11|8|8|0|0' ]] || die 'restart no-op assertion: SQL changed'
alive restart-no-op
stop_watch
log 'PASS: watched cumulative session persisted through append, partial completion, no-op and checkpoint restart'
