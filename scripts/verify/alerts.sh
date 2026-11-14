#!/usr/bin/env bash
set -Eeuo pipefail
# Public HTTP oracle; SQL supplies persisted observations and independently counts rows.
# ALERTS_VERIFY_CONTROL=1 disables the production low-cache predicate in a disposable copy.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
HEAVY=/Users/zeross/agent-ops/scripts/heavy.sh
ARTIFACT_DIR=${ALERTS_VERIFY_ARTIFACT_DIR:-$ROOT/.agent-runs/W19i-alerts-517/$(date +%s)-$$}
mkdir -p "$ARTIFACT_DIR"
ARTIFACT_DIR=$(cd "$ARTIFACT_DIR" && pwd)
mkdir -p "$ARTIFACT_DIR/buildx"
export BUILDX_CONFIG="$ARTIFACT_DIR/buildx"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/alerts-517.XXXXXX")
PROJECT="alerts-517-$(id -u)-$$"
# Ask the OS for available ephemeral ports; Compose still detects races at bind time.
read -r API_PORT PG_PORT STUB_PORT <<<"$(python3 - <<'PY'
import socket
s=[socket.socket() for _ in range(3)]
for v in s: v.bind(('127.0.0.1',0))
print(*(v.getsockname()[1] for v in s))
PY
)"
API="http://127.0.0.1:$API_PORT"
ORG=51700000-0000-4000-8000-000000000001
REPO=51700000-0000-4000-8000-000000000002
FAILURES=0
COMPOSE=(docker compose -p "$PROJECT" -f "$TMP/compose.yml")
dc() { "$HEAVY" "${COMPOSE[@]}" "$@"; }
log() { printf 'alerts: %s\n' "$*"; }
fatal() { log "FAIL: $*"; exit 1; }
assert_eq() {
  printf '| %s | %s | %s |\n' "$1" "$2" "$3" >>"$ARTIFACT_DIR/comparisons.md"
  if [[ $2 == "$3" ]]; then log "PASS: $1 expected=$2 actual=$3";
  else log "FAIL: $1 expected=$2 actual=$3"; FAILURES=$((FAILURES+1)); fi
}
cleanup() {
  local status=$? containers volumes networks images
  trap - EXIT INT TERM
  dc logs --no-color api >"$ARTIFACT_DIR/api.log" 2>&1 || true
  dc down --volumes --remove-orphans --rmi local >"$ARTIFACT_DIR/cleanup.log" 2>&1 || status=1
  if [[ -n ${STUB_PID:-} ]]; then kill "$STUB_PID" 2>/dev/null || true; wait "$STUB_PID" 2>/dev/null || true; fi
  containers=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT") || status=1
  volumes=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT") || status=1
  networks=$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT") || status=1
  images=$(docker image ls -q "$PROJECT-api") || status=1
  printf 'containers=%s\nvolumes=%s\nnetworks=%s\nimages=%s\n' "${containers:-none}" "${volumes:-none}" "${networks:-none}" "${images:-none}" | tee "$ARTIFACT_DIR/resources.txt"
  if [[ -n $containers || -n $volumes || -n $networks || -n $images ]]; then status=1; log 'FAIL: cleanup owned resources';
  else log 'PASS: cleanup zero owned containers, volumes, networks and images'; fi
  rm -rf "$TMP"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
printf '| Assertion | Expected | Actual |\n| --- | --- | --- |\n' >"$ARTIFACT_DIR/comparisons.md"
git -C "$ROOT" rev-parse HEAD >"$ARTIFACT_DIR/tested-sha.txt"
printf 'project=%s control=%s\n' "$PROJECT" "${ALERTS_VERIFY_CONTROL:-0}" >"$ARTIFACT_DIR/config.txt"
CONTEXT="$ROOT/api"
if [[ ${ALERTS_VERIFY_CONTROL:-0} == 1 ]]; then
  cp -R "$ROOT/api" "$TMP/api-control"
  CONTEXT="$TMP/api-control"
  python3 - "$CONTEXT/modules/alert/evaluator.go" <<'PY'
import sys
from pathlib import Path
p=Path(sys.argv[1]); s=p.read_text(); old='denominator > 0 && hit < e.thresholds.MinCacheHit'
assert s.count(old)==1, 'production mutation target changed'
p.write_text(s.replace(old, 'denominator > 0 && hit < e.thresholds.MinCacheHit && false'))
PY
  log 'CONTROL: production low-cache predicate disabled; fixture and oracle unchanged'
fi
cat >"$TMP/compose.yml" <<YAML
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: agent_ops
      POSTGRES_PASSWORD: agent_ops_dev
      POSTGRES_DB: agent_ops
    ports: ["127.0.0.1:$PG_PORT:5432"]
    volumes: ["data:/var/lib/postgresql/data"]
  api:
    build:
      context: $CONTEXT
    environment:
      DATABASE_URL: postgres://agent_ops:agent_ops_dev@postgres:5432/agent_ops?sslmode=disable
      HTTP_ADDR: :8080
      JWT_SECRET: alerts-verification-only
      REPOSITORY_ENCRYPTION_KEY: "01234567890123456789012345678901"
      SCM_REPOSITORIES_JSON: '[]'
      VERIFY_SLACK_URL: http://host.docker.internal:$STUB_PORT/hook
    ports: ["127.0.0.1:$API_PORT:8080"]
volumes:
  data:
YAML
# No provider, authentication, database or runtime is stubbed.
cat >"$TMP/channel.py" <<'PY'
import json, sys
from pathlib import Path
from http.server import BaseHTTPRequestHandler, HTTPServer
port, directory=int(sys.argv[1]), Path(sys.argv[2])
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers()
    def do_POST(self):
        body=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        code=502 if (directory/'fail-channel').exists() else 204
        with (directory/'channel.jsonl').open('a') as f:
            f.write(json.dumps(dict(path=self.path,content_type=self.headers.get('Content-Type'),body=body,status=code))+'\n')
        self.send_response(code); self.end_headers()
    def log_message(self,*args): pass
HTTPServer(('0.0.0.0',port),H).serve_forever()
PY
: >"$ARTIFACT_DIR/channel.jsonl"
python3 "$TMP/channel.py" "$STUB_PORT" "$ARTIFACT_DIR" >"$ARTIFACT_DIR/channel.log" 2>&1 & STUB_PID=$!
for _ in $(seq 1 30); do curl -fsS "http://127.0.0.1:$STUB_PORT/" >/dev/null 2>&1 && break; sleep .1; done
curl -fsS "http://127.0.0.1:$STUB_PORT/" >/dev/null || fatal 'stub readiness'
sql() { printf '%s\n' "$1" >>"$ARTIFACT_DIR/sql.log"; dc exec -T postgres psql -U agent_ops -d agent_ops -At -v ON_ERROR_STOP=1 -c "$1" | tr -d '\r'; }
ready() {
  for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && return; sleep 1; done
  fatal 'API readiness'
}
http() {
  local method=$1 path=$2 data=${3:-} code
  printf '%s %s %s\n' "$method" "$path" "$data" >>"$ARTIFACT_DIR/http-requests.log"
  code=$(curl --max-time 15 -sS -o "$ARTIFACT_DIR/response.json" -w '%{http_code}' -X "$method" -H "Authorization: Bearer ${JWT:-}" -H 'Content-Type: application/json' ${data:+--data-binary "$data"} "$API/api/v1/$path")
  cp "$ARTIFACT_DIR/response.json" "$ARTIFACT_DIR/http-$((HTTP_SEQ+=1)).json"
  [[ $code == 200 || $code == 201 ]] || fatal "HTTP $method $path status=$code $(cat "$ARTIFACT_DIR/response.json")"
  log "PASS: HTTP $method $path status=$code"
}
HTTP_SEQ=0
dc up -d postgres
for _ in $(seq 1 60); do dc exec -T postgres pg_isready -U agent_ops >/dev/null 2>&1 && break; sleep 1; done
dc build api
dc up -d api
ready
dc exec -T api sh -c "printf '%s\\n' alerts-verification-password | /bootstrap-account --email alerts-517@example.invalid"
ACCOUNT=$(sql "SELECT id FROM accounts WHERE email='alerts-517@example.invalid'")
sql "INSERT INTO organizations(id,name) VALUES ('$ORG','Alerts verification'); INSERT INTO organization_memberships(id,organization_id,account_id,role) VALUES (gen_random_uuid(),'$ORG','$ACCOUNT','owner'); INSERT INTO repositories(id,provider,full_name,webhook_secret_env_ref,organization_id) SELECT ('51700000-0000-4000-8000-' || lpad(n::text,12,'0'))::uuid,'github','verify/alerts-'||n,'UNUSED','$ORG' FROM generate_series(2,102) n;" >/dev/null
http POST auth/login '{"email":"alerts-517@example.invalid","password":"alerts-verification-password"}'
JWT=$(jq -r .token "$ARTIFACT_DIR/response.json")
http POST auth/select-organization "{\"organization_id\":\"$ORG\"}"
JWT=$(jq -r .token "$ARTIFACT_DIR/response.json")
http POST budgets "{\"repository_id\":\"$REPO\",\"token_limit\":100,\"starts_at\":\"2020-01-01T00:00:00Z\",\"ends_at\":\"2099-01-01T00:00:00Z\"}"
BUDGET=$(jq -r .data.id "$ARTIFACT_DIR/response.json")
http GET "budgets/$BUDGET"
assert_eq 'budget persisted token limit' 100 "$(jq -r .data.token_limit "$ARTIFACT_DIR/response.json")"
assert_eq 'no alert before persisted usage' 0 "$(sql "SELECT count(*) FROM alert_occurrences")"
http PUT alerts/thresholds '{"min_cache_hit":0.8,"silent_seconds":300,"pr_stale_seconds":86400}'
http POST alerts/channels '{"kind":"slack","enabled":true,"slack_webhook_url_env_ref":"VERIFY_SLACK_URL"}'
# Fixed inputs in all modes. 101 repositories exceed the actual production scan limit (100).
sql "INSERT INTO agent_runs(id,repository_id,wave,worker,model,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,started_at,ended_at,updated_at) SELECT ('51800000-0000-4000-8000-' || right(id::text,12))::uuid,id,'W19','worker-'||full_name,'fixture',81,0,0,0,0,'2026-11-14T00:00:00Z','2026-11-14T00:01:00Z','2026-11-14T00:01:00Z' FROM repositories WHERE organization_id='$ORG';" >/dev/null
http GET "agent-runs?repository_id=$REPO"
assert_eq 'HTTP persisted run tokens cross 80 percent budget' 81 "$(jq -r '.data[0].input_tokens' "$ARTIFACT_DIR/response.json")"
# Restart makes the normal initial tick observe the complete fixture immediately.
dc restart api
ready
for _ in $(seq 1 30); do [[ $(sql "SELECT count(*) FROM alert_occurrences") == 100 ]] && break; sleep 1; done
assert_eq 'low-cache occurrence first scan' 100 "$(sql "SELECT count(*) FROM alert_occurrences WHERE rule='low_cache_hit'")"
# Fail fast at the normal assertion under the production mutation, never at an induced crash.
if [[ ${ALERTS_VERIFY_CONTROL:-0} == 1 && $FAILURES -gt 0 ]]; then exit 1; fi
# Next page must advance without restart. One-minute interval is hard-coded in production.
touch "$ARTIFACT_DIR/fail-channel"
for _ in $(seq 1 75); do [[ $(sql "SELECT count(*) FROM alert_occurrences WHERE rule='low_cache_hit'") == 101 ]] && break; sleep 1; done
assert_eq 'scan progress beyond 100 repositories' 101 "$(sql "SELECT count(*) FROM alert_occurrences WHERE rule='low_cache_hit'")"
assert_eq 'channel failure recorded' 1 "$(sql "SELECT count(*) FROM alert_deliveries WHERE success=false")"
rm "$ARTIFACT_DIR/fail-channel"
# Restart with identical stored inputs must retry failed delivery and keep occurrences unique.
dc restart api
ready
sleep 3
# Allow two full pages so the failed occurrence's repository has been revisited.
for _ in $(seq 1 125); do [[ $(sql "SELECT count(*) FROM alert_deliveries WHERE success=true") == 101 ]] && break; sleep 1; done
assert_eq 'channel failure retried after recovery' 101 "$(sql "SELECT count(*) FROM alert_deliveries WHERE success=true")"
assert_eq 'restart preserves one occurrence per key' '101|101' "$(sql "SELECT count(*)||'|'||count(DISTINCT alert_key) FROM alert_occurrences WHERE rule='low_cache_hit'")"
http GET 'alerts/history?per_page=1&page=1'
assert_eq 'HTTP alert history scan total' 101 "$(jq -r .meta.total "$ARTIFACT_DIR/response.json")"
# Budget threshold is a separate contract from cache-hit alerts. Never substitute low_cache_hit.
assert_eq 'budget threshold alert from persisted usage' 1 "$(sql "SELECT count(*) FROM alert_occurrences WHERE rule <> 'low_cache_hit' AND repository='verify/alerts-2'")"
# Cross 100% independently with 20 additional cached tokens (no new low-cache subject).
sql "INSERT INTO agent_runs(id,repository_id,wave,worker,model,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,started_at,ended_at,updated_at) VALUES ('51900000-0000-4000-8000-000000000002','$REPO','W19','budget-second','fixture',0,20,0,0,0,'2026-11-14T00:02:00Z','2026-11-14T00:03:00Z','2026-11-14T00:03:00Z');" >/dev/null
assert_eq 'persisted tokens cross 100 percent budget' 101 "$(sql "SELECT sum(input_tokens+cached_input_tokens) FROM agent_runs WHERE repository_id='$REPO'")"
dc restart api
ready
sleep 3
assert_eq 'budget 80 and 100 percent occurrences' 2 "$(sql "SELECT count(*) FROM alert_occurrences WHERE rule <> 'low_cache_hit' AND repository='verify/alerts-2'")"
# The README promises a new occurrence after a healthy observation and a new failure.
sql "UPDATE agent_runs SET input_tokens=0,cached_input_tokens=81 WHERE id='51800000-0000-4000-8000-000000000002';" >/dev/null
dc restart api
ready
sleep 3
assert_eq 'recovered observation adds no occurrence' 1 "$(sql "SELECT count(*) FROM alert_occurrences WHERE rule='low_cache_hit' AND repository='verify/alerts-2'")"
sql "UPDATE agent_runs SET input_tokens=81,cached_input_tokens=0 WHERE id='51800000-0000-4000-8000-000000000002';" >/dev/null
dc restart api
ready
sleep 3
assert_eq 'new incident after recovery gets new occurrence' 2 "$(sql "SELECT count(*) FROM alert_occurrences WHERE rule='low_cache_hit' AND repository='verify/alerts-2'")"
sql "SELECT rule,repository,subject,alert_key FROM alert_occurrences ORDER BY repository,rule; SELECT occurrence_id,channel,success,error FROM alert_deliveries ORDER BY occurrence_id;" >"$ARTIFACT_DIR/persisted-results.txt"
python3 - "$ARTIFACT_DIR/channel.jsonl" <<'PY'
import json,sys
rows=[json.loads(s) for s in open(sys.argv[1])]
assert rows and all(r['path']=='/hook' and r['content_type']=='application/json' and r['body']['text']=='low_cache_hit\ncache hit below threshold' for r in rows), 'channel HTTP request contract'
assert any(r['status']==204 for r in rows), 'successful external channel delivery'
assert any(r['status']==502 for r in rows), 'failing external channel delivery'
print('alerts: PASS: exact channel path, JSON payload, success and failure boundary')
PY
log "Completed strict acceptance failures=$FAILURES artifacts=$ARTIFACT_DIR"
[[ $FAILURES == 0 ]]
