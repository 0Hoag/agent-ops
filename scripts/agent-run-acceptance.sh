#!/usr/bin/env bash
set -Eeuo pipefail

# End-to-end acceptance for the persisted agent-run HTTP contract. The API is
# built from this worktree; PostgreSQL is the only owned container.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-run-acceptance-$(id -u)-$$"
API_PORT=${AGENT_RUN_ACCEPTANCE_API_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
PG_PORT=${AGENT_RUN_ACCEPTANCE_PG_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
TOKEN='acceptance-secret'
REPO='a1000000-0000-4000-8000-000000000001'
API="http://127.0.0.1:$API_PORT"
DB="postgres://agent_ops:agent_ops_dev@127.0.0.1:$PG_PORT/agent_ops?sslmode=disable"
API_PID=''
TMP=''
MUTATION=${AGENT_RUN_ACCEPTANCE_MUTATION:-}

die() { echo "acceptance: FAIL: $*" >&2; exit 1; }
log() { echo "acceptance: $*"; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  [[ -z "$API_PID" ]] || { kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; }
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null 2>&1 || true
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then log "cleanup FAILED: owned containers remain"; status=1; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then log "cleanup FAILED: owned volumes remain"; status=1; fi
  [[ -z "$TMP" ]] || rm -rf "$TMP" 2>/dev/null || true
  [[ $status -eq 0 ]] && log "cleanup verified: no owned containers or volumes ($PROJECT)"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

[[ "$PG_PORT" != 5432 ]] || die "refusing PostgreSQL host port 5432"
[[ -z "$(lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null || true)" ]] || die "API port $API_PORT is busy"
[[ -z "$(lsof -nP -iTCP:"$PG_PORT" -sTCP:LISTEN 2>/dev/null || true)" ]] || die "PostgreSQL port $PG_PORT is busy"

POSTGRES_HOST_PORT="$PG_PORT" POSTGRES_PUBLISH=127.0.0.1 docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" up -d postgres >/dev/null
for _ in $(seq 1 60); do
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break
  sleep 1
done
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null || die "PostgreSQL did not become ready"

start_api() {
  local dir=${1:-$ROOT}
  export AGENT_RUN_TOKEN="$TOKEN" AGENT_RUN_MODEL_PRICES='{"gpt-accept":{"input_per_million":1,"cached_input_per_million":2,"cache_write_per_million":3,"output_per_million":4,"reasoning_per_million":5}}' \
    DATABASE_URL="$DB" HTTP_ADDR=":$API_PORT" SCM_REPOSITORIES_JSON='[]'
    (cd "$dir/api" && go run .) >"${TMP:-/tmp}/api.log" 2>&1 &
  API_PID=$!
  for _ in $(seq 1 60); do curl -fsS "$API/readyz" >/dev/null 2>&1 && return; sleep 1; done
  sed -n '1,160p' "${TMP:-/tmp}/api.log" >&2; die "API did not become ready"
}
post() { curl -sS -w '\n%{http_code}' -H 'Content-Type: application/json' -H "X-Agent-Run-Token: $TOKEN" "$API/api/v1/agent-runs" -d "$1"; }
expect_code() { local got=$1 want=$2 label=$3; [[ "${got##*$'\n'}" == "$want" ]] || die "$label: expected HTTP $want, got $got"; }

TMP=$(mktemp -d)
export GOMODCACHE="$TMP/gomod" GOCACHE="$TMP/gocache"
mkdir -p "$GOMODCACHE" "$GOCACHE"
start_api
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 <<SQL
INSERT INTO repositories (id, provider, full_name, webhook_secret_env_ref) VALUES ('$REPO', 'github', 'acceptance/wire', 'ACCEPTANCE_SECRET');
SQL
BASE='{"repository_id":"'"$REPO"'","wave":"W3G","worker":"wire","model":"gpt-accept","issue_numbers":[191,192],"pr_number":3191,"input_tokens":100,"cached_input_tokens":20,"cache_write_tokens":30,"output_tokens":40,"reasoning_tokens":50,"started_at":"2026-10-24T09:00:00Z","ended_at":"2026-10-24T09:05:00Z"}'
response=$(post "$BASE"); expect_code "$response" 201 "authenticated POST"
id=$(echo "$response" | sed '$d' | jq -r .data.id); [[ "$id" != null && -n "$id" ]] || die "POST did not return persisted identity"
negative=$(post "${BASE/\"input_tokens\":100/\"input_tokens\":-1}"); expect_code "$negative" 400 "negative token validation"
response=$(post "${BASE/W3G/W3G-distractor}"); expect_code "$response" 201 "distractor POST"
DISTRACTOR='{"repository_id":"'"$REPO"'","wave":"OTHER","worker":"other","model":"gpt-accept","issue_numbers":[999],"pr_number":3999,"input_tokens":7,"cached_input_tokens":8,"cache_write_tokens":9,"output_tokens":10,"reasoning_tokens":11,"started_at":"2026-10-24T10:00:00Z","ended_at":"2026-10-24T10:01:00Z"}'
expect_code "$(post "$DISTRACTOR")" 201 "filter distractor POST"

row=$(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT repository_id,wave,worker,model,issue_numbers::text,pr_number,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"'),to_char(ended_at AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM agent_runs WHERE id='$id'")
expected="$REPO|W3G|wire|gpt-accept|[191, 192]|3191|100|20|30|40|50|2026-10-24T09:00:00Z|2026-10-24T09:05:00Z"
[[ "$row" == "$expected" ]] || die "exact SQL normalization mismatch: expected '$expected', got '$row'"

list=$(curl -fsS "$API/api/v1/agent-runs?repository_id=$REPO&wave=W3G&issue_number=191")
[[ "$(echo "$list" | jq '.data|length')" == 1 ]] || die "repository/wave/issue filters returned wrong count: $list"
[[ "$(echo "$list" | jq -r '.data[0].issue_numbers')" == WzE5MSwgMTkyXQ== ]] || die "filtered identity mismatch: $list"
agg=$(curl -fsS "$API/api/v1/agent-runs/aggregate?repository_id=$REPO&wave=W3G&issue_number=191")
echo "$agg" | jq -e '.data|length == 2 and any(.[]; .wave == "W3G" and .worker == "wire" and .issue_number == 191 and .runs == 1 and .input_tokens == 100 and .cached_input_tokens == 20 and .cache_write_tokens == 30 and .output_tokens == 40 and .reasoning_tokens == 50 and .cost_usd == 0.00064)' >/dev/null || die "aggregate exact tokens/cost mismatch: $agg"
log "wire/SQL acceptance passed: id=$id, filtered count=1, aggregate groups=2, cost=0.00064"

if [[ -n "$MUTATION" ]]; then
  die "mutation mode must be run as a disposable control, not after the success assertions"
fi

log "success path complete"
