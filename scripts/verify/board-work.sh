#!/usr/bin/env bash
set -Eeuo pipefail

# Real HTTP acceptance for Board/Work ingestion, reads, and provider sync.
# BOARD_WORK_VERIFY_CONTROL=1 rebuilds an isolated API copy with the status guard mutated.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${BOARD_WORK_VERIFY_ARTIFACT_DIR:-/Users/zeross/agent-ops/.agent-runs/W19e-board-work-513}
RUN_DIR="$ROOT/.agent-runs/board-work-verify-$$"
mkdir -p "$ARTIFACT_DIR" "$RUN_DIR"/{cache,modcache,buildx}
export GOCACHE="$RUN_DIR/cache" GOMODCACHE="$RUN_DIR/modcache" BUILDX_CONFIG="$RUN_DIR/buildx"

PROJECT="agent-ops-board-work-$(id -u)-$$"
API_PORT=${BOARD_WORK_VERIFY_API_PORT:-$((20400 + RANDOM % 500))}
PG_PORT=${BOARD_WORK_VERIFY_PG_PORT:-$((17400 + RANDOM % 500))}
STUB_PORT=${BOARD_WORK_VERIFY_STUB_PORT:-$((18400 + RANDOM % 500))}
API="http://127.0.0.1:$API_PORT"
STUB="http://host.docker.internal:$STUB_PORT"
ORG=51300000-0000-4000-8000-000000000001
REPO=verify/board-work
REPO_ID=51300000-0000-4000-8000-000000000002
SECRET=board-work-webhook-secret
TOKEN=board-work-provider-token
KEY=01234567890123456789012345678901
ACCOUNT="board-work-$(date +%s)-$$@example.invalid"
PASSWORD=board-work-password
TMP=$(mktemp -d "${TMPDIR:-/tmp}/board-work.XXXXXX")

COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=board-work-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" SYNC_INTERVAL=1h \
  SMOKE_WEBHOOK_SECRET="$SECRET" SMOKE_BACKFILL_TOKEN="$TOKEN" SMOKE_GITLAB_FIXTURE_URL="$STUB" \
  SCM_REPOSITORIES_JSON="[{\"provider\":\"github\",\"full_name\":\"$REPO\",\"organization_id\":\"$ORG\",\"webhook_secret_env\":\"SMOKE_WEBHOOK_SECRET\",\"token_env\":\"SMOKE_BACKFILL_TOKEN\",\"base_url_env\":\"SMOKE_GITLAB_FIXTURE_URL\"}]" \
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
API_COMPOSE=("${COMPOSE[@]}")
BOOT_COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=board-work-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" SCM_REPOSITORIES_JSON=[] \
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")

log() { printf 'board-work: %s\n' "$*"; }
fail() { log "FAIL: $*"; exit 1; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  kill "${STUB_PID:-}" 2>/dev/null || true
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || status=1
  local containers volumes networks
  containers=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")
  volumes=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT")
  networks=$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT")
  if [[ -n $containers || -n $volumes || -n $networks ]]; then
    log "FAIL: cleanup left owned resources (containers=${containers:-none} volumes=${volumes:-none} networks=${networks:-none})"
    status=1
  else
    log "PASS: cleanup verified zero owned containers, volumes, and networks ($PROJECT)"
  fi
  rm -rf "$TMP" "$RUN_DIR"
  exit "$status"
}
trap cleanup EXIT INT TERM

sql() { "${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -v ON_ERROR_STOP=1 -c "$1" | tr -d '\r'; }
request() { CODE=$(curl -sS -o "$TMP/body" -w '%{http_code}' "$@"); BODY=$(tr '\n' ' ' <"$TMP/body"); }
expect_http() { [[ $CODE == "$1" ]] || fail "$2 expected HTTP $1 got $CODE body=$BODY"; log "PASS: $2 HTTP=$CODE"; }
json() { jq -e "$1" "$TMP/body" >/dev/null || fail "$2 body=$BODY"; log "PASS: $2"; }
sign() { printf 'sha256=%s' "$(openssl dgst -sha256 -hmac "$2" "$1" | awk '{print $2}')"; }
webhook() { local event=$1 delivery=$2 file=$3; request -X POST -H 'Content-Type: application/json' -H "X-GitHub-Event: $event" -H "X-GitHub-Delivery: $delivery" -H "X-Hub-Signature-256: $(sign "$file" "$SECRET")" --data-binary "@$file" "$API/api/v1/webhooks/github"; }

cat >"$TMP/provider.py" <<'PY'
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
port, log = int(sys.argv[1]), sys.argv[2]
class H(BaseHTTPRequestHandler):
  def do_GET(self):
    with open(log, 'a') as f: f.write(json.dumps({'path': self.path, 'auth': self.headers.get('Authorization')})+'\n')
    if self.path == '/health': body=b'ok'; code=200
    elif self.path.startswith('/repos/verify/board-work') and '/issues' not in self.path and '/pulls' not in self.path and '/actions' not in self.path:
      body=b'{"full_name":"verify/board-work"}'; code=200
    elif '/issues?' in self.path:
      body=b'[{"id":101,"number":101,"title":"sync issue","state":"open","updated_at":"2026-11-14T13:00:00Z","labels":[{"name":"status:blocked"}]}]'; code=200
    elif '/pulls?' in self.path:
      body=b'[{"id":201,"number":201,"title":"sync pull","state":"open","body":"Closes #101","updated_at":"2026-11-14T13:01:00Z","head":{"sha":"sha-sync","ref":"sync-branch"}}]'; code=200
    elif '/actions/runs?' in self.path:
      body=b'{"workflow_runs":[{"id":301,"status":"completed","conclusion":"failure","head_sha":"sha-sync","run_started_at":"2026-11-14T13:02:00Z","updated_at":"2026-11-14T13:03:00Z","pull_requests":[{"number":201}]}]}'; code=200
    else: body=b'[]'; code=200
    self.send_response(code); self.send_header('Content-Type','application/json'); self.end_headers(); self.wfile.write(body)
  def log_message(self, *args): pass
HTTPServer(('0.0.0.0', port), H).serve_forever()
PY
python3 "$TMP/provider.py" "$STUB_PORT" "$ARTIFACT_DIR/provider.jsonl" >"$ARTIFACT_DIR/provider.log" 2>&1 & STUB_PID=$!
for _ in $(seq 1 30); do curl -fsS "http://127.0.0.1:$STUB_PORT/health" >/dev/null && break; sleep .1; done

"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d postgres
for _ in $(seq 1 90); do "${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
"${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null || fail 'postgres did not become ready'
"$ROOT/scripts/heavy.sh" "${BOOT_COMPOSE[@]}" up -d api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null || fail 'bootstrap api did not become ready'
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "INSERT INTO organizations(id,name) VALUES ('$ORG','Board Work verification') ON CONFLICT (id) DO NOTHING;" >/dev/null
"${BOOT_COMPOSE[@]}" stop api >/dev/null
"${BOOT_COMPOSE[@]}" rm -f api >/dev/null
if [[ ${BOARD_WORK_VERIFY_CONTROL:-0} == 1 ]]; then
  cp -R "$ROOT/api" "$RUN_DIR/api-control"
  python3 - "$RUN_DIR/api-control/pkg/handlers/scm/provider.go" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1]); s = p.read_text()
old = 'result.Issues[i].Status = IssueStatus(strings.ReplaceAll(v, " ", "_"))'
new = 'result.Issues[i].Status = IssueStatus(v); result.Issues[i].Status = StatusTodo'
if old not in s: raise SystemExit('negative-control mutation target missing')
p.write_text(s.replace(old, new, 1))
PY
  cat >"$RUN_DIR/compose.control.yml" <<EOF
services:
  api:
    build:
      context: $RUN_DIR/api-control
EOF
  API_COMPOSE+=( -f "$RUN_DIR/compose.control.yml" )
  log 'negative control: rebuilt API with status:review mapped to todo'
fi
if [[ ${BOARD_WORK_VERIFY_CONTROL:-0} == 1 ]]; then
  "$ROOT/scripts/heavy.sh" "${API_COMPOSE[@]}" build --no-cache api
fi
"$ROOT/scripts/heavy.sh" "${API_COMPOSE[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null || fail 'api did not become ready'
printf '%s\n' "$PASSWORD" | "${COMPOSE[@]}" exec -T api /bootstrap-account --email "$ACCOUNT" >/dev/null
ACCOUNT_ID=$(sql "SELECT id FROM accounts WHERE email='$ACCOUNT'")
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "INSERT INTO organization_memberships(id,organization_id,account_id,role) VALUES ('51300000-0000-4000-8000-000000000003','$ORG','$ACCOUNT_ID','owner') ON CONFLICT DO NOTHING;" >/dev/null
request -H 'Content-Type: application/json' -d "{\"email\":\"$ACCOUNT\",\"password\":\"$PASSWORD\"}" "$API/api/v1/auth/login"; expect_http 200 'login'; TOKEN_JWT=$(jq -r .token "$TMP/body")
request -H "Authorization: Bearer $TOKEN_JWT" "$API/api/v1/auth/select-organization" -H 'Content-Type: application/json' -d "{\"organization_id\":\"$ORG\"}"; expect_http 200 'organization selection'; TOKEN_JWT=$(jq -r .token "$TMP/body")
REPO_ID=$(sql "SELECT id FROM repositories WHERE full_name='$REPO'")
[[ -n $REPO_ID ]] || fail 'configured repository was not persisted'
log "PASS: configured repository persisted id=$REPO_ID"

cat >"$TMP/issue.json" <<EOF
{"repository":{"full_name":"$REPO"},"issue":{"id":1001,"number":101,"title":"Webhook issue","state":"open","updated_at":"2026-11-14T12:00:00Z","labels":[{"name":"status:review"}]}}
EOF
cat >"$TMP/pr.json" <<EOF
{"action":"opened","repository":{"full_name":"$REPO"},"pull_request":{"id":2001,"number":201,"title":"Webhook PR","state":"open","body":"Closes #101","updated_at":"2026-11-14T12:01:00Z","head":{"sha":"sha-webhook","ref":"feature/webhook"}}}
EOF
cat >"$TMP/run.json" <<EOF
{"action":"completed","repository":{"full_name":"$REPO"},"workflow_run":{"id":3001,"status":"completed","conclusion":"success","head_sha":"sha-webhook","run_started_at":"2026-11-14T12:02:00Z","updated_at":"2026-11-14T12:03:00Z","pull_requests":[{"number":201}]}}
EOF
webhook issues issue-513-1 "$TMP/issue.json"; expect_http 200 'signed issue webhook'; [[ $(sql "SELECT status FROM issues WHERE repository_id='$REPO_ID' AND number=101;") == review ]] || fail 'issue status mapping not persisted'
webhook pull_request pr-513-1 "$TMP/pr.json"; expect_http 200 'signed pull request webhook'; [[ $(sql "SELECT head_sha || '|' || closes::text FROM pull_requests WHERE repository_id='$REPO_ID' AND number=201;") == 'sha-webhook|[101]' ]] || fail 'linked PR closing issue not persisted'
webhook workflow_run run-513-1 "$TMP/run.json"; expect_http 200 'signed workflow webhook'; [[ $(sql "SELECT conclusion FROM pipeline_runs WHERE repository_id='$REPO_ID' AND external_id='3001';") == success ]] || fail 'pipeline conclusion not persisted'
log 'PASS: webhook rows assert status=review, closes=[101], conclusion=success'

request -H "Authorization: Bearer $TOKEN_JWT" "$API/api/v1/board?repository_id=$REPO_ID&page=1&per_page=1"; expect_http 200 'board pagination'; json '.meta.page == 1 and .meta.per_page == 1 and .meta.total == 1 and .data[0].status == "review" and .data[0].pull_requests[0].pipeline_runs[0].conclusion == "success"' 'board response includes mapped issue, linked PR, and CI'
request -H "Authorization: Bearer $TOKEN_JWT" "$API/api/v1/work?repository_id=$REPO_ID&state=open&q=Webhook&page=1"; expect_http 200 'work state/title filters'; json '.meta.total == 2 and ([.data[].title] | sort) == ["Webhook PR","Webhook issue"]' 'work filters and pagination'
request -H "Authorization: Bearer $TOKEN_JWT" "$API/api/v1/work/pr/201?repository_id=$REPO_ID"; expect_http 200 'work PR detail'; json '.item.kind == "pr" and .item.number == 201 and .item.ci_status == "success"' 'work detail exposes CI conclusion'

request -H "Authorization: Bearer $TOKEN_JWT" -X POST "$API/api/v1/repositories/$REPO_ID/sync"; expect_http 200 'authenticated provider sync'; json '.data.last_sync_status == "success"' 'sync reports success'; [[ $(sql "SELECT status FROM issues WHERE repository_id='$REPO_ID' AND number=101;") == blocked ]] || fail 'sync status update was not persisted'; [[ $(sql "SELECT head_sha || '|' || closes::text FROM pull_requests WHERE repository_id='$REPO_ID' AND number=201;") == 'sha-sync|[101]' ]] || fail 'sync PR update was not persisted'; [[ $(sql "SELECT conclusion FROM pipeline_runs WHERE repository_id='$REPO_ID' AND external_id='301';") == failure ]] || fail 'sync CI conclusion was not persisted'; log 'PASS: provider HTTP sync updated stored issue, PR, and CI rows'
grep -q '"path": "/repos/verify/board-work/issues?per_page=100&page=1&state=all"' "$ARTIFACT_DIR/provider.jsonl" || fail 'provider issue pagination/query was not exercised'; grep -q '"path": "/repos/verify/board-work/pulls?per_page=100&page=1&state=all"' "$ARTIFACT_DIR/provider.jsonl" || fail 'provider PR pagination/query was not exercised'; grep -q '"path": "/repos/verify/board-work/actions/runs?per_page=100&page=1"' "$ARTIFACT_DIR/provider.jsonl" || fail 'provider CI pagination/query was not exercised'; log 'PASS: provider request table matched exact URI/query and token boundary'

if [[ ${BOARD_WORK_VERIFY_CONTROL:-0} == 1 ]]; then
  fail 'negative control unexpectedly passed the identical status assertion'
fi
log "PASS: all #513 acceptance checks completed source=$(git -C "$ROOT" rev-parse HEAD)"
