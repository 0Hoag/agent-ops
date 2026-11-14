#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# This verifier serializes each heavy operation itself. Invoke this script
# directly; wrapping the whole script in scripts/heavy.sh is unsupported.
ARTIFACT_DIR=${INSIGHTS_VERIFY_ARTIFACT_DIR:-/Users/zeross/agent-ops/.agent-runs/W19h-insights-516}
RUN_DIR="$ROOT/.agent-runs/insights-verify-$$"
mkdir -p "$ARTIFACT_DIR" "$RUN_DIR"/{cache,modcache,buildx}
export GOCACHE="$RUN_DIR/cache" GOMODCACHE="$RUN_DIR/modcache" BUILDX_CONFIG="$RUN_DIR/buildx"
PROJECT="agent-ops-insights-$(id -u)-$$"
API_PORT=${INSIGHTS_VERIFY_API_PORT:-$((21400 + RANDOM % 400))}; PG_PORT=${INSIGHTS_VERIFY_PG_PORT:-$((18400 + RANDOM % 400))}
API="http://127.0.0.1:$API_PORT"; KEY=01234567890123456789012345678901
ORG=51600000-0000-4000-8000-000000000001; REPO=51600000-0000-4000-8000-000000000002
ACCOUNT="insights-verify-$(date +%s)-$$@example.invalid"; PASSWORD=insights-verify-password
TMP=$(mktemp -d "${TMPDIR:-/tmp}/insights-verify.XXXXXX")
COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" JWT_SECRET=insights-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" SCM_REPOSITORIES_JSON='[]' docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
log() { printf 'insights: %s\n' "$*" | tee -a "$ARTIFACT_DIR/commands.log"; }
fail() { log "FAIL: $*"; exit 1; }
sql() { "$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" exec -T postgres psql -At -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "$1" | tr -d '\r'; }
cleanup() { local rc=$?; trap - EXIT INT TERM; "$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || rc=1; local c v n; c=$("$ROOT/scripts/heavy.sh" docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null || true); v=$("$ROOT/scripts/heavy.sh" docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null || true); n=$("$ROOT/scripts/heavy.sh" docker network ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null || true); [[ -z "$c$v$n" ]] || { log "FAIL: cleanup left owned resources containers=${c:-none} volumes=${v:-none} networks=${n:-none}"; rc=1; }; cp "$RUN_DIR/commands.log" "$ARTIFACT_DIR/commands.log" 2>/dev/null || true; rm -rf "$TMP" "$RUN_DIR"; [[ $rc -eq 0 ]] && log "PASS: cleanup verified zero owned containers, volumes, and networks ($PROJECT)"; exit "$rc"; }
trap cleanup EXIT INT TERM
request() { CODE=$(curl -sS -o "$TMP/body" -w '%{http_code}' "$@"); BODY=$(tr '\n' ' ' <"$TMP/body"); }
expect() { [[ "$CODE" == "$1" ]] || fail "$2 expected HTTP $1 got $CODE body=$BODY"; log "PASS: $2 HTTP=$CODE"; }
check() { jq -e "$1" "$TMP/body" >/dev/null || fail "$2 body=$BODY"; log "PASS: $2"; }

"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null || fail 'API did not become ready'
log "PASS: isolated real API/Postgres stack ready project=$PROJECT api=$API"
printf '%s\n' "$PASSWORD" >"$TMP/password"
"$ROOT/scripts/heavy.sh" bash -c "cat '$TMP/password' | ${COMPOSE[*]} exec -T api /bootstrap-account --email '$ACCOUNT'" >/dev/null
ACCOUNT_ID=$(sql "SELECT id FROM accounts WHERE email='$ACCOUNT'")
[[ -n "$ACCOUNT_ID" ]] || fail 'bootstrap account was not persisted'
cat >"$TMP/fixtures.sql" <<SQL
INSERT INTO organizations(id,name) VALUES ('$ORG','Insights verification');
INSERT INTO organization_memberships(id,organization_id,account_id,role) VALUES ('51600000-0000-4000-8000-000000000003','$ORG',(SELECT id FROM accounts WHERE email='$ACCOUNT'),'owner');
INSERT INTO repositories(id,provider,full_name,webhook_secret_env_ref,organization_id) VALUES ('$REPO','github','fixture/insights-516','FIXTURE','$ORG');
INSERT INTO agent_run_price_versions(id,organization_id,model,effective_date,input_per_million,cached_input_per_million,cache_write_per_million,output_per_million,reasoning_per_million) VALUES ('51600000-0000-4000-8000-000000000010','$ORG','priced-model','2026-01-01',10,1,2,20,5),('51600000-0000-4000-8000-000000000011','$ORG','priced-model','2026-11-01',20,2,4,40,10);
INSERT INTO issues(id,repository_id,number,title,state,status,labels,created_at,updated_at) VALUES ('51600000-0000-4000-8000-000000000020','$REPO',516,'Area issue','closed','done','[{"name":"area:platform"}]','2026-11-10T00:00:00Z','2026-11-11T00:00:00Z');
INSERT INTO pull_requests(id,repository_id,number,title,state,head_sha,branch,closes,merged_at,created_at,updated_at) VALUES ('51600000-0000-4000-8000-000000000021','$REPO',516,'Merged PR','closed','sha-516','verify/516','[516]','2026-11-12T12:00:00Z','2026-11-10T00:00:00Z','2026-11-12T12:00:00Z'),('51600000-0000-4000-8000-000000000022','$REPO',517,'Stale PR','open','sha-517','stale','[]',NULL,'2026-11-01T00:00:00Z','2026-11-11T00:00:00Z'),('51600000-0000-4000-8000-000000000023','$REPO',518,'Failing PR','open','sha-518','failing','[]',NULL,'2026-11-01T00:00:00Z','2026-11-13T00:00:00Z');
INSERT INTO pipeline_runs(id,repository_id,external_id,pr_number,sha,status,conclusion,started_at,finished_at,updated_at) VALUES ('51600000-0000-4000-8000-000000000030','$REPO','run-516',516,'sha-516','completed','success','2026-11-11T10:00:00Z','2026-11-11T10:05:00Z','2026-11-11T10:05:00Z'),('51600000-0000-4000-8000-000000000031','$REPO','run-518',518,'sha-518','completed','failure','2026-11-12T00:00:00Z','2026-11-12T00:05:00Z','2026-11-13T00:00:00Z');
INSERT INTO agent_runs(id,repository_id,wave,worker,model,issue_numbers,pr_number,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,cost_usd,started_at,ended_at,harness,session_id,billing_type,branch) VALUES ('51600000-0000-4000-8000-000000000040','$REPO','W19','fixture','priced-model','[516]',516,100,50,10,20,5,NULL,'2026-11-10T12:00:00Z','2026-11-10T12:10:00Z','codex','overview-current','api','verify/516'),('51600000-0000-4000-8000-000000000041','$REPO','W18','fixture','priced-model','[]',NULL,40,0,0,10,0,NULL,'2026-11-13T00:00:00Z','2026-11-13T00:05:00Z','codex','overview-unlinked','api',''),('51600000-0000-4000-8000-000000000042','$REPO','W18','fixture','priced-model','[]',516,10,0,0,5,0,NULL,'2026-11-08T00:00:00Z','2026-11-08T00:05:00Z','codex','overview-previous','api','verify/516'),('51600000-0000-4000-8000-000000000043','$REPO','W19','fixture','priced-model','[]',516,100,0,0,10,0,NULL,'2026-01-31T23:30:00Z','2026-01-31T23:35:00Z','codex','month-end-a','api','verify/516'),('51600000-0000-4000-8000-000000000044','$REPO','W19','fixture','priced-model','[]',516,200,100,0,20,0,NULL,'2026-02-01T00:30:00Z','2026-02-01T00:35:00Z','codex','month-end-b','subscription','verify/516');
INSERT INTO webhook_events(id,repository_id,provider,event_type,delivery_id,payload,received_at) VALUES ('51600000-0000-4000-8000-000000000050','$REPO','github','issues','516-issue-start','{"issue":{"number":516,"state":"open","labels":[{"name":"status:in_progress"}]}}','2026-11-10T01:00:00Z'),('51600000-0000-4000-8000-000000000051','$REPO','github','issues','516-issue-review','{"issue":{"number":516,"state":"open","labels":[{"name":"status:review"}]}}','2026-11-10T02:00:00Z'),('51600000-0000-4000-8000-000000000052','$REPO','github','issues','516-issue-return','{"issue":{"number":516,"state":"open","labels":[{"name":"status:in_progress"}]}}','2026-11-10T03:00:00Z'),('51600000-0000-4000-8000-000000000053','$REPO','github','issues','516-issue-done','{"issue":{"number":516,"state":"closed","labels":[]}}','2026-11-12T12:00:00Z'),('51600000-0000-4000-8000-000000000054','$REPO','github','workflow_run','516-run','{"workflow_run":{"id":516,"head_sha":"sha-516","conclusion":"success","run_started_at":"2026-11-11T10:00:00Z","updated_at":"2026-11-11T10:05:00Z","pull_requests":[{"number":516}]}}','2026-11-11T10:05:00Z');
SQL
"$ROOT/scripts/heavy.sh" bash -c "cat '$TMP/fixtures.sql' | ${COMPOSE[*]} exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1" >/dev/null
[[ "$(sql "SELECT count(*) FROM organization_memberships WHERE organization_id='$ORG' AND account_id='$ACCOUNT_ID'")" == 1 ]] || fail 'organization membership fixture was not persisted'
log 'PASS: seeded/stored real fixtures in Postgres'
request -H 'Content-Type: application/json' -d "{\"email\":\"$ACCOUNT\",\"password\":\"$PASSWORD\"}" "$API/api/v1/auth/login"; expect 200 'login'; TOKEN=$(jq -r .token "$TMP/body")
request -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "{\"organization_id\":\"$ORG\"}" "$API/api/v1/auth/select-organization"; expect 200 'organization selection'; TOKEN=$(jq -r .token "$TMP/body")
request -H "Authorization: Bearer $TOKEN" "$API/api/v1/overview?repository_id=$REPO&from=2026-11-10&to=2026-11-14"; expect 200 'overview HTTP response'
check '.data.current.runs == 2 and .data.current.input_tokens == 140 and .data.current.output_tokens == 30 and .data.current.cache_read_tokens == 50 and (.data.current.cache_hit_rate == (50/190)) and .data.current.merged_prs == 1 and .data.current.rework_rate == 1 and .data.current.ci_first_pass_rate == 1 and .data.previous.runs == 1 and ([.data.attention[].kind] | sort) == ["failing_ci","stale_pr","unlinked_run"]' 'overview arithmetic, period delta, and 24h/72h attention boundaries'
log 'CHECK TABLE: overview current runs=2 input=140 cache_hit_rate=50/190 merged_prs=1 rework_rate=1 ci_first_pass_rate=1; previous runs=1; attention=stale_pr(72h),unlinked_run(24h),failing_ci(24h)'
request -H "Authorization: Bearer $TOKEN" "$API/api/v1/insights/analytics?repository_id=$REPO&from=2026-01-31&to=2026-02-01&period=daily&group_by=model"; expect 200 'analytics HTTP response'
check '.data.buckets | length == 2 and .[0].start == "2026-01-31T00:00:00Z" and .[0].input_tokens == 100 and .[1].start == "2026-02-01T00:00:00Z" and .[1].input_tokens == 200 and .[1].cached_input_tokens == 100 and .[1].actual_cost_usd == null and .[1].equivalent_cost_usd != null' 'UTC month-end daily buckets and subscription cost split'
log 'CHECK TABLE: analytics Jan31 input=100; Feb01 input=200 cached=100 actual_cost=null equivalent_cost=historical price'
request -H "Authorization: Bearer $TOKEN" "$API/api/v1/overview?repository_id=$REPO&from=2025-01-01&to=2025-01-02"; expect 200 'empty-data overview HTTP response'; check '.data.current.runs == 0 and .data.current.input_tokens == 0 and .data.current.models_without_price == [] and .data.attention == []' 'empty-data results are explicit zeros and empty arrays'
if [[ ${INSIGHTS_VERIFY_CONTROL:-0} == 1 ]]; then
  cp -R "$ROOT/api" "$RUN_DIR/api-control"
  python3 - "$RUN_DIR/api-control/modules/insight/overview.go" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1]); s = p.read_text(); old = 'out.CacheHitRate = float64(out.CacheReadTokens) / float64(denom)'
if old not in s: raise SystemExit('negative-control mutation target missing')
p.write_text(s.replace(old, 'out.CacheHitRate = 0', 1))
PY
  cat >"$RUN_DIR/compose.control.yml" <<EOF
services:
  api:
    build:
      context: $RUN_DIR/api-control
EOF
  CONTROL_COMPOSE=("${COMPOSE[@]}" -f "$RUN_DIR/compose.control.yml"); "$ROOT/scripts/heavy.sh" "${CONTROL_COMPOSE[@]}" up -d --build api
  for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
  request -H "Authorization: Bearer $TOKEN" "$API/api/v1/overview?repository_id=$REPO&from=2026-11-10&to=2026-11-14"; expect 200 'negative-control overview HTTP response'
  if jq -e '.data.current.cache_hit_rate == (50/190)' "$TMP/body" >/dev/null; then fail 'negative control unexpectedly passed identical cache-rate assertion'; fi
  log 'PASS: negative control failed intended cache-rate assertion with identical HTTP inputs'
fi
log "PASS: all #516 checks completed tested_sha=$(git -C "$ROOT" rev-parse HEAD)"
