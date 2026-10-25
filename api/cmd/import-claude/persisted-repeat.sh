#!/usr/bin/env bash
set -Eeuo pipefail

API_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
REPO_ROOT=$(cd "$API_DIR/.." && pwd)
PROJECT="claude-import-repeat-$(id -u)-$$"
API_PORT=${CLAUDE_IMPORT_API_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
PG_PORT=${CLAUDE_IMPORT_PG_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
TOKEN=claude-import-acceptance-secret
REPOSITORY=ca100000-0000-4000-8000-000000000001
API="http://127.0.0.1:$API_PORT"
TMP=$(mktemp -d)
API_PID=
compose() { POSTGRES_HOST_PORT="$PG_PORT" POSTGRES_PUBLISH=127.0.0.1 docker compose -p "$PROJECT" -f "$REPO_ROOT/docker-compose.yml" "$@"; }
cleanup() {
  local status=$? containers volumes listener
  trap - EXIT INT TERM
  if [[ -n "$API_PID" ]]; then kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; fi
  if [[ $status -ne 0 && -f "$TMP/api.log" ]]; then printf '%s\n' '--- fixture API log ---' >&2; cat "$TMP/api.log" >&2; fi
  compose down --volumes --remove-orphans >/dev/null || status=1
  containers=$(docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' || true)
  volumes=$(docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' || true)
  listener=$(lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null || true)
  [[ -z "$containers" && -z "$volumes" ]] || { printf 'cleanup FAILED: containers=%s volumes=%s\n' "$containers" "$volumes" >&2; status=1; }
  [[ -z "$listener" ]] || { printf 'cleanup FAILED: API listener remains: %s\n' "$listener" >&2; status=1; }
  rm -rf "$TMP" || status=1
  printf 'cleanup verified: project=%s containers=absent volumes=absent API-listener=absent host-5432=untouched\n' "$PROJECT"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
[[ "$PG_PORT" != 5432 && "$API_PORT" != 5432 ]] || { printf 'refusing host port 5432\n' >&2; exit 1; }
[[ -z "$(lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null || true)" ]] || { printf 'API port busy: %s\n' "$API_PORT" >&2; exit 1; }
[[ -z "$(lsof -nP -iTCP:"$PG_PORT" -sTCP:LISTEN 2>/dev/null || true)" ]] || { printf 'Postgres port busy: %s\n' "$PG_PORT" >&2; exit 1; }
printf 'fixture project=%s API port=%s PostgreSQL port=%s (not 5432)\n' "$PROJECT" "$API_PORT" "$PG_PORT"
(cd "$API_DIR" && go build -o "$TMP/import-claude" ./cmd/import-claude)
(cd "$API_DIR" && go build -o "$TMP/api" .)
compose up -d postgres >/dev/null
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
AGENT_RUN_MODEL_PRICES='{"claude-test":{"input_per_million":1,"cached_input_per_million":2,"cache_write_per_million":3,"output_per_million":4,"reasoning_per_million":5}}' DATABASE_URL="postgres://agent_ops:agent_ops_dev@127.0.0.1:$PG_PORT/agent_ops?sslmode=disable" HTTP_ADDR=":$API_PORT" AGENT_RUN_TOKEN="$TOKEN" SCM_REPOSITORIES_JSON='[]' "$TMP/api" >"$TMP/api.log" 2>&1 & API_PID=$!
for _ in $(seq 1 60); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz"
compose exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops -c "INSERT INTO repositories (id,provider,full_name,webhook_secret_env_ref) VALUES ('$REPOSITORY','github','acceptance/claude-import','CLAUDE_IMPORT_SECRET')"
CHECKPOINT="$TMP/checkpoint.json"
run_import() { AGENT_RUN_TOKEN="$TOKEN" "$TMP/import-claude" -path "$API_DIR/cmd/import-claude/testdata/synthetic.jsonl" -checkpoint "$CHECKPOINT" -repository-id "$REPOSITORY" -endpoint "$API/api/v1/agent-runs"; }
snapshot() {
  compose exec -T postgres psql -At -F '|' -U agent_ops -d agent_ops -c "SELECT count(*),coalesce(sum(input_tokens),0),coalesce(sum(cached_input_tokens),0),coalesce(sum(cache_write_tokens),0),coalesce(sum(output_tokens),0),coalesce(sum(reasoning_tokens),0) FROM agent_runs WHERE repository_id='$REPOSITORY' AND wave='W3d' AND worker='claude-import'"
}
printf 'before count|input|cache-read|cache-write|output|reasoning = '
before=$(snapshot); printf '%s\n' "$before"
run_import
printf 'after-first count|input|cache-read|cache-write|output|reasoning = '
first=$(snapshot); printf '%s\n' "$first"
run_import
printf 'after-second count|input|cache-read|cache-write|output|reasoning = '
second=$(snapshot); printf '%s\n' "$second"
[[ "$before" == '0|0|0|0|0|0' ]] || { printf 'unexpected non-empty initial snapshot\n' >&2; exit 1; }
[[ "$first" == '2|17|8|8|12|0' ]] || { printf 'unexpected first snapshot: %s\n' "$first" >&2; exit 1; }
[[ "$second" == "$first" ]] || { printf 'second import changed persisted snapshot\n' >&2; exit 1; }
printf 'persisted-repeat assertion: PASS (separate importer invocations, exact snapshot unchanged)\n'
