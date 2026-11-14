#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${REPOSITORIES_VERIFY_ARTIFACT_DIR:-$ROOT/.agent-runs/W19-repositories-511}
RUN_DIR="$ROOT/.agent-runs/repositories-verify-$$"
mkdir -p "$ARTIFACT_DIR" "$RUN_DIR"/{cache,modcache,buildx}
export GOCACHE="$RUN_DIR/cache" GOMODCACHE="$RUN_DIR/modcache" BUILDX_CONFIG="$RUN_DIR/buildx"

PROJECT="agent-ops-repositories-$(id -u)-$$"
API_PORT=${REPOSITORIES_VERIFY_API_PORT:-$((19080 + RANDOM % 700))}
PG_PORT=${REPOSITORIES_VERIFY_PG_PORT:-$((16432 + RANDOM % 700))}
STUB_PORT=${REPOSITORIES_VERIFY_STUB_PORT:-$((18081 + RANDOM % 700))}
ORG=51100000-0000-4000-8000-000000000001
ACCOUNT="repositories-verify-$(date +%s)-$$@example.invalid"
PASSWORD='repositories-verify-password'
KEY=01234567890123456789012345678901
TOKEN=synthetic-repositories-token
API="http://127.0.0.1:$API_PORT"
STUB="http://host.docker.internal:$STUB_PORT"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/repositories-verify.XXXXXX")
COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=repository-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" SYNC_INTERVAL=1s SCM_REPOSITORIES_JSON=[] \
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
CONFIG_COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=repository-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" SYNC_INTERVAL=1s \
  SCM_REPOSITORIES_JSON="[{\"provider\":\"gitlab\",\"full_name\":\"fixture/group/scheduled\",\"organization_id\":\"$ORG\",\"token_env\":\"REPOSITORIES_VERIFY_TOKEN\",\"webhook_secret_env\":\"REPOSITORIES_VERIFY_SECRET\",\"base_url_env\":\"REPOSITORIES_VERIFY_BASE\"}]" \
  REPOSITORIES_VERIFY_TOKEN="$TOKEN" REPOSITORIES_VERIFY_SECRET=synthetic-secret REPOSITORIES_VERIFY_BASE="$STUB" \
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")

log() { printf 'repositories: %s\n' "$*"; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  kill "${STUB_PID:-}" 2>/dev/null || true
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || status=1
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then status=1; log "cleanup failed: owned containers remain"; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then status=1; log "cleanup failed: owned volumes remain"; fi
  rm -rf "$TMP" "$RUN_DIR"
  [[ $status -eq 0 ]] && log "cleanup verified: no owned containers or volumes ($PROJECT)"
  exit "$status"
}
trap cleanup EXIT INT TERM

cat >"$TMP/stub.py" <<'PY'
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
port, log = int(sys.argv[1]), sys.argv[2]
class H(BaseHTTPRequestHandler):
  def do_GET(self):
    with open(log, 'a') as f: f.write(json.dumps({'path': self.path, 'token': self.headers.get('PRIVATE-TOKEN'), 'auth': self.headers.get('Authorization')})+'\n')
    if self.path == '/health': self.send_response(200); self.end_headers(); return
    if 'bad-token' in (self.headers.get('PRIVATE-TOKEN') or self.headers.get('Authorization') or ''): self.send_response(401); self.end_headers(); return
    if self.path.startswith('/repos/'):
      body=b'{"full_name":"fixture/github-repository"}'
    elif self.path.startswith('/api/v4/projects/') and '/issues' not in self.path and '/merge_requests' not in self.path and '/pipelines' not in self.path:
      body=b'{"name":"scheduled"}'
    elif self.path.startswith('/api/v4/projects/') and '/issues?' in self.path:
      body=b'[{"id":1,"iid":511,"title":"fixture issue","state":"opened","updated_at":"2026-01-01T00:00:00Z","labels":["Status::Review"]}]'
    else: body=b'[]'
    self.send_response(200); self.send_header('Content-Type','application/json'); self.end_headers(); self.wfile.write(body)
  def log_message(self, *args): pass
HTTPServer(('0.0.0.0', port), H).serve_forever()
PY
python3 "$TMP/stub.py" "$STUB_PORT" "$ARTIFACT_DIR/provider.jsonl" >"$ARTIFACT_DIR/provider.log" 2>&1 & STUB_PID=$!
for _ in $(seq 1 30); do curl -fsS "http://127.0.0.1:$STUB_PORT/health" >/dev/null && break; sleep .1; done

"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d postgres
for _ in $(seq 1 60); do "${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
"${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null
"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null && break; sleep 1; done
printf '%s\n' "$PASSWORD" | "${COMPOSE[@]}" exec -T api /bootstrap-account --email "$ACCOUNT" >/dev/null
ACCOUNT_ID=$("${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -c "SELECT id FROM accounts WHERE email='$ACCOUNT'" | tr -d '\r\n')
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 >/dev/null <<SQL
INSERT INTO organizations(id,name) VALUES ('$ORG','Repository verification');
INSERT INTO organization_memberships(id,organization_id,account_id,role) VALUES (gen_random_uuid(),'$ORG','$ACCOUNT_ID','owner');
SQL
"${COMPOSE[@]}" stop api >/dev/null
"${COMPOSE[@]}" rm -f api >/dev/null
"$ROOT/scripts/heavy.sh" "${CONFIG_COMPOSE[@]}" up -d api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null && break; sleep 1; done

request() { CODE=$(curl -sS -o "$TMP/body" -w '%{http_code}' "$@"); }
check() { [[ "$CODE" == "$1" ]] || { log "FAIL: $2 expected HTTP $1 got $CODE: $(tr '\n' ' ' <"$TMP/body")"; exit 1; }; log "PASS: $2 (HTTP $CODE)"; }
json() { python3 -c 'import json,sys; value=json.load(sys.stdin); [value:=value[key] for key in sys.argv[1].split(".")]; print(value)' "$1" <"$TMP/body"; }

request -H 'Content-Type: application/json' -d '{"email":"'$ACCOUNT'","password":"'$PASSWORD'"}' "$API/api/v1/auth/login"; check 200 'login'; LOGIN=$(json token)
request -H "Authorization: Bearer $LOGIN" -H 'Content-Type: application/json' -d '{"organization_id":"'$ORG'"}' "$API/api/v1/auth/select-organization"; check 200 'select organization'; SESSION=$(json token)
H=(-H "Authorization: Bearer $SESSION")

request "${H[@]}" -H 'Content-Type: application/json' -d '{"provider":"github","full_name":"fixture/github-repository","base_url":"'$STUB'","token":"'$TOKEN'"}' "$API/api/v1/repositories/test"; check 200 'GitHub connection succeeds'; grep -q 'github-repository' "$TMP/body"
request "${H[@]}" -H 'Content-Type: application/json' -d '{"provider":"gitlab","full_name":"fixture/group/project","base_url":"'$STUB'","token":"'$TOKEN'"}' "$API/api/v1/repositories/test"; check 200 'GitLab connection succeeds'
request "${H[@]}" -H 'Content-Type: application/json' -d '{"provider":"github","full_name":"fixture/github-repository","base_url":"'$STUB'","token":"bad-token"}' "$API/api/v1/repositories/test"; check 422 'invalid provider token is reported'
request "${H[@]}" -H 'Content-Type: application/json' -d '{"provider":"gitlab","full_name":"fixture/group/project","base_url":"http://127.0.0.1:1","token":"'$TOKEN'"}' "$API/api/v1/repositories/test"; check 502 'unreachable provider is reported'

request "${H[@]}" -H 'Content-Type: application/json' -d '{"provider":"github","full_name":"fixture/github-repository","webhook_secret":"github-secret","backfill_token":"github-token"}' "$API/api/v1/repositories"; check 201 'GitHub repository create'; GITHUB_ID=$(json 'data.id'); grep -q 'github-secret' "$TMP/body"
request "${H[@]}" -X PATCH -H 'Content-Type: application/json' -d '{"backfill_token":"github-token-updated"}' "$API/api/v1/repositories/$GITHUB_ID"; check 200 'GitHub repository update'
request "${H[@]}" -X DELETE "$API/api/v1/repositories/$GITHUB_ID"; check 204 'GitHub repository delete'
request "${H[@]}" -H 'Content-Type: application/json' -d '{"provider":"gitlab","full_name":"fixture/group/manual","gitlab_base_url":"'$STUB'","webhook_secret":"gitlab-secret","backfill_token":"'$TOKEN'","status_mappings":{"Status::Review":"blocked"}}' "$API/api/v1/repositories"; check 201 'GitLab repository create'; GITLAB_ID=$(json 'data.id')
request "${H[@]}" -X PATCH -H 'Content-Type: application/json' -d '{"status_mappings":{"Status::Review":"review"}}' "$API/api/v1/repositories/$GITLAB_ID"; check 200 'GitLab repository update'; grep -q 'review' "$TMP/body"
request "${H[@]}" -X POST "$API/api/v1/repositories/$GITLAB_ID/sync"; check 200 'manual GitLab sync'; grep -q '"last_sync_status":"success"' "$TMP/body"; grep -q '"last_synced_at"' "$TMP/body"
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -c "SELECT last_sync_status FROM repositories WHERE id='$GITLAB_ID'" | grep -qx success
request "${H[@]}" -X DELETE "$API/api/v1/repositories/$GITLAB_ID"; check 204 'GitLab repository delete'

for _ in $(seq 1 20); do scheduled=$("${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -c "SELECT count(*) FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='fixture/group/scheduled' AND i.number=511" | tr -d '[:space:]'); [[ "$scheduled" == 1 ]] && break; sleep 1; done
[[ "$scheduled" == 1 ]] || { log 'FAIL: scheduled sync did not persist fixture issue'; exit 1; }; log 'PASS: scheduled sync persisted fixture issue'
grep -q 'issues?per_page=100&page=1&state=all' "$ARTIFACT_DIR/provider.jsonl" || { log 'FAIL: provider issue request URI/query missing'; exit 1; }; log 'PASS: provider request URI/query recorded'

cp "$ARTIFACT_DIR/provider.jsonl" "$ARTIFACT_DIR/provider-$(git -C "$ROOT" rev-parse --short HEAD).jsonl"
log 'PASS: repository CRUD, provider errors, manual sync, scheduled sync, persistence, and provider request assertions completed'
