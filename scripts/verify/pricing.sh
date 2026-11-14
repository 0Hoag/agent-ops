#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${PRICING_VERIFY_ARTIFACT_DIR:-$ROOT/.agent-runs/W19g-pricing-515}
RUN_DIR="$ROOT/.agent-runs/pricing-verify-$$"
mkdir -p "$ARTIFACT_DIR" "$RUN_DIR"
mkdir -p "$RUN_DIR"/{cache,modcache,buildx}
export GOCACHE="$RUN_DIR/cache" GOMODCACHE="$RUN_DIR/modcache" BUILDX_CONFIG="$RUN_DIR/buildx"
PROJECT="agent-ops-pricing-$(id -u)-$$"
API_PORT=${PRICING_VERIFY_API_PORT:-$((20600 + RANDOM % 300))}
PG_PORT=${PRICING_VERIFY_PG_PORT:-$((17600 + RANDOM % 300))}
API="http://127.0.0.1:$API_PORT"
ORG=51500000-0000-4000-8000-000000000001
REPO=51500000-0000-4000-8000-000000000002
ACCOUNT="pricing-verify-$(date +%s)-$$@example.invalid"
PASSWORD=pricing-verify-password
TOKEN=pricing-verify-token
KEY=01234567890123456789012345678901
TMP=$(mktemp -d "${TMPDIR:-/tmp}/pricing-verify.XXXXXX")
COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" JWT_SECRET=pricing-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" AGENT_RUN_TOKEN="$TOKEN" EXCHANGE_RATE_VND_PER_USD=25000 SCM_REPOSITORIES_JSON='[]' docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")

log() { printf 'pricing: %s\n' "$*" | tee -a "$ARTIFACT_DIR/commands.log" >&2; }
fail() { log "FAIL: $*"; exit 1; }
cleanup() {
  local rc=$?; trap - EXIT INT TERM
  "$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || rc=1
  local c v n
  c=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")
  v=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT")
  n=$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT")
  [[ -z $c && -z $v && -z $n ]] || { log "FAIL: cleanup left owned resources containers=${c:-none} volumes=${v:-none} networks=${n:-none}"; rc=1; }
  cp "$RUN_DIR/commands.log" "$ARTIFACT_DIR/commands.log" 2>/dev/null || true
  rm -rf "$TMP" "$RUN_DIR"
  [[ $rc -eq 0 ]] && log "PASS: cleanup verified zero owned containers, volumes, and networks ($PROJECT)"
  exit "$rc"
}
trap cleanup EXIT INT TERM
sql() { "$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -v ON_ERROR_STOP=1 -c "$1" | tr -d '\r'; }
request() { CODE=$(curl -sS -o "$TMP/body" -w '%{http_code}' "$@"); BODY=$(tr '\n' ' ' <"$TMP/body"); }
expect() { [[ $CODE == "$1" ]] || fail "$2 expected HTTP $1 got $CODE body=$BODY"; log "PASS: $2 HTTP=$CODE"; }
json() { jq -e "$1" "$TMP/body" >/dev/null || fail "$2 body=$BODY"; log "PASS: $2"; }

"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d postgres
for _ in $(seq 1 60); do "${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
"${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null || fail 'postgres did not become ready'
"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null || fail 'api did not become ready'

sql "INSERT INTO organizations(id,name) VALUES ('$ORG','Pricing verification'); INSERT INTO repositories(id,organization_id,provider,full_name,webhook_secret_env_ref) VALUES ('$REPO','$ORG','github','fixture/pricing-515','PRICING_VERIFY_SECRET');"
printf '%s\n' "$PASSWORD" | "${COMPOSE[@]}" exec -T api /bootstrap-account --email "$ACCOUNT" >/dev/null
ACCOUNT_ID=$(sql "SELECT id FROM accounts WHERE email='$ACCOUNT'")
sql "INSERT INTO organization_memberships(id,organization_id,account_id,role) VALUES ('51500000-0000-4000-8000-000000000003','$ORG','$ACCOUNT_ID','owner');"
request -H 'Content-Type: application/json' -d "{\"email\":\"$ACCOUNT\",\"password\":\"$PASSWORD\"}" "$API/api/v1/auth/login"; expect 200 login; SESSION=$(jq -r .token "$TMP/body")
request -H "Authorization: Bearer $SESSION" -H 'Content-Type: application/json' -d "{\"organization_id\":\"$ORG\"}" "$API/api/v1/auth/select-organization"; expect 200 'organization selection'; SESSION=$(jq -r .token "$TMP/body")
H=(-H "Authorization: Bearer $SESSION")

price() { request "${H[@]}" -H 'Content-Type: application/json' -d "$2" "$API/api/v1/agent-run-prices"; expect 201 "$1"; printf '%s' "$(jq -r '.data.id' "$TMP/body")"; }
P1=$(price 'create model-a April price' '{"model":"model-a","effective_date":"2026-04-01","input_per_million":1,"cached_input_per_million":2,"cache_write_per_million":3,"output_per_million":4,"reasoning_per_million":5}')
P2=$(price 'create model-a May price' '{"model":"model-a","effective_date":"2026-05-01","input_per_million":10,"cached_input_per_million":20,"cache_write_per_million":30,"output_per_million":40,"reasoning_per_million":50}')
P3=$(price 'create model-b price' '{"model":"model-b","effective_date":"2026-04-01","input_per_million":2,"cached_input_per_million":0,"cache_write_per_million":0,"output_per_million":6,"reasoning_per_million":0}')
request "${H[@]}" "$API/api/v1/agent-run-prices"; expect 200 'list price versions'; json '(.data|length) == 3 and ([.data[] | .model+":"+(.effective_date|split("T")[0])]|sort) == ["model-a:2026-04-01","model-a:2026-05-01","model-b:2026-04-01"]' 'effective-date price rows';

run() { local model=$1 day=$2 billing=$3 input=$4 cached=$5 write=$6 output=$7 reasoning=$8; local worker="pricing-${model}-${day}-${billing}"; request -X POST -H "X-Agent-Run-Token: $TOKEN" -H 'Content-Type: application/json' -d "{\"repository_id\":\"$REPO\",\"wave\":\"W19\",\"worker\":\"$worker\",\"model\":\"$model\",\"started_at\":\"${day}T12:00:00Z\",\"ended_at\":\"${day}T12:01:00Z\",\"input_tokens\":$input,\"cached_input_tokens\":$cached,\"cache_write_tokens\":$write,\"output_tokens\":$output,\"reasoning_tokens\":$reasoning,\"billing_type\":\"$billing\"}" "$API/api/v1/agent-runs"; expect 201 "ingest $model $day $billing"; }
run model-a 2026-04-10 api 1000000 0 0 0 0
run model-a 2026-05-10 api 1000000 0 0 0 0
run model-a 2026-05-10 subscription 0 0 0 1000000 0
run model-b 2026-04-10 api 1000000 0 0 1000000 0
run unknown 2026-06-10 api 100 0 0 0 0

request "${H[@]}" "$API/api/v1/agent-runs/aggregate?repository_id=$REPO"; expect 200 'aggregate five runs'; json '[.data|length] == [5]' 'aggregate has five independent groups'
python3 - "$TMP/body" <<'PY' >"$ARTIFACT_DIR/check-table.tsv"
import json, sys
rows=json.load(open(sys.argv[1]))['data']; rows=sorted(rows,key=lambda x:(x['worker'],x['wave'],x.get('billed_cost_usd') if x.get('billed_cost_usd') is not None else -1))
for r in rows: print(r['worker'],r['runs'],r.get('cost_usd'),r.get('equivalent_cost_usd'),r.get('billed_cost_usd'),r.get('cost_vnd','null'))
PY
cat "$ARTIFACT_DIR/check-table.tsv" | tee -a "$ARTIFACT_DIR/commands.log"
python3 - "$TMP/body" <<'PY'
import json,sys,math
d=json.load(open(sys.argv[1]))['data']
known=[x for x in d if x['worker'].startswith('pricing-')]
assert len(known)==5
vals=sorted([(x.get('cost_usd'),x.get('equivalent_cost_usd'),x.get('billed_cost_usd')) for x in known], key=lambda x: (x[0] is not None, x[0] or 0))
expected=[(None,None,None),(1,1,1),(8,8,8),(10,10,10),(40,40,0)]
for got,want in zip(vals,expected):
  assert got[0] is None if want[0] is None else math.isclose(got[0],want[0],rel_tol=1e-9)
  if want[0] is not None: assert math.isclose(got[1],want[1],rel_tol=1e-9) and math.isclose(got[2],want[2],rel_tol=1e-9)
assert any(math.isclose(x.get('cost_vnd',-1),25000,rel_tol=1e-9) for x in known)
PY
log 'PASS: five independently hand-calculated runs: effective dates, null missing price, API/subscription billing, USD-to-VND conversion'

if [[ ${PRICING_VERIFY_CONTROL:-0} == 1 ]]; then
  sql "UPDATE agent_run_price_versions SET input_per_million=400 WHERE id='$P2';"
  request "${H[@]}" "$API/api/v1/agent-runs/aggregate?repository_id=$REPO"; expect 200 'control aggregate HTTP'
  set +e; python3 - "$TMP/body" <<'PY'
import json,sys,math
d=json.load(open(sys.argv[1]))['data']; assert any(math.isclose(x.get('cost_usd',-1),.01,rel_tol=1e-9) for x in d)
PY
  rc=$?; set -e; [[ $rc -ne 0 ]] || fail 'negative control unexpectedly passed mutated price assertion'; log 'PASS: negative control failed intended cost assertion with identical HTTP inputs after persisted price mutation'
fi
log "PASS: all #515 acceptance checks completed source=$(git -C "$ROOT" rev-parse HEAD)"
