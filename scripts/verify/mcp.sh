#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
HEAVY=/Users/zeross/agent-ops/scripts/heavy.sh
EVIDENCE=${MCP_VERIFY_ARTIFACT_DIR:-/Users/zeross/agent-ops/.agent-runs/W19k-mcp-519}
RUN=$(mktemp -d "${TMPDIR:-/tmp}/mcp-519.XXXXXX")
PROJECT="mcp-519-$(id -u)-$$"
API_PORT=${MCP_VERIFY_API_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
PG_PORT=${MCP_VERIFY_PG_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
API="http://127.0.0.1:$API_PORT"; DB="postgres://agent_ops:agent_ops_dev@127.0.0.1:$PG_PORT/agent_ops?sslmode=disable"
JWT_SECRET=mcp-519-disposable-secret; KEY=01234567890123456789012345678901
ORG=51900000-0000-4000-8000-000000000001; ACCOUNT=51900000-0000-4000-8000-000000000002
REPO=51900000-0000-4000-8000-000000000003; WAVE=51900000-0000-4000-8000-000000000004
ISSUE=51900000-0000-4000-8000-000000000005; AGENT_RUN=51900000-0000-4000-8000-000000000006
PASSWORD=mcp-519-fixture-password
mkdir -p "$EVIDENCE"; chmod 700 "$EVIDENCE"
compose=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" JWT_SECRET="$JWT_SECRET" REPOSITORY_ENCRYPTION_KEY="$KEY" SCM_REPOSITORIES_JSON='[]' docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
sql() { "$HEAVY" "${compose[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -v ON_ERROR_STOP=1 -c "$1"; }
cleanup() {
  local rc=$? left; trap - EXIT INT TERM
  "$HEAVY" "${compose[@]}" down --volumes --remove-orphans >"$EVIDENCE/cleanup.log" 2>&1 || rc=1
  for kind in container volume network; do
    case "$kind" in
      container) left=$($HEAVY docker ps -aq --filter "label=com.docker.compose.project=$PROJECT");;
      volume) left=$($HEAVY docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT");;
      network) left=$($HEAVY docker network ls -q --filter "label=com.docker.compose.project=$PROJECT");;
    esac
    [[ -z "$left" ]] || { echo "FAIL: owned $kind remains: $left" | tee -a "$EVIDENCE/cleanup.log"; rc=1; }
  done
  echo "cleanup verified: no owned containers, volumes, or networks ($PROJECT)" | tee -a "$EVIDENCE/cleanup.log"
  rm -rf "$RUN"; exit "$rc"
}
trap cleanup EXIT INT TERM
printf 'tested_sha=%s project=%s\n' "$(git -C "$ROOT" rev-parse HEAD)" "$PROJECT" >"$EVIDENCE/source.txt"

"$HEAVY" "${compose[@]}" up -d postgres >/dev/null
for _ in $(seq 1 60); do sql 'SELECT 1' >/dev/null 2>&1 && break; sleep 1; done
sql 'SELECT 1' >/dev/null
cp -R "$ROOT/api" "$RUN/api-source"
sed -i.bak 's/^go 1\.26\.11$/go 1.26.2/' "$RUN/api-source/go.mod"
GOCACHE="$RUN/cache" GOMODCACHE="$RUN/modcache" GOPATH="$RUN/gopath" GOTOOLCHAIN=local bash -c "cd '$RUN/api-source' && go build -o '$RUN/agentops' ./cmd/agentops && go build -o '$RUN/bootstrap-account' ./cmd/bootstrap-account"
DATABASE_URL="$DB" HTTP_ADDR=":$API_PORT" JWT_SECRET="$JWT_SECRET" REPOSITORY_ENCRYPTION_KEY="$KEY" SCM_REPOSITORIES_JSON='[]' "$RUN/agentops" >"$RUN/api.log" 2>&1 & API_PID=$!
cleanup_api() { kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; }
trap 'cleanup_api; cleanup' EXIT INT TERM
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null
printf '%s\n' "$PASSWORD" | DATABASE_URL="$DB" "$RUN/bootstrap-account" --email mcp-519@example.invalid >/dev/null
sql "INSERT INTO organizations(id,name) VALUES ('$ORG','MCP 519 fixture'); INSERT INTO organization_memberships(id,organization_id,account_id,role) SELECT gen_random_uuid(),'$ORG',id,'owner' FROM accounts WHERE email='mcp-519@example.invalid'; INSERT INTO repositories(id,provider,full_name,webhook_secret_env_ref,organization_id) VALUES ('$REPO','github','fixture/mcp-519','MCP_519_SECRET','$ORG'); INSERT INTO issues(id,repository_id,number,title,state,status,labels,assignee) VALUES ('$ISSUE','$REPO',519,'MCP verification issue','open','review','[\"status:review\"]','fixture'); INSERT INTO waves(id,organization_id,name,brief,state) VALUES ('$WAVE','$ORG','W19k','MCP verification','in_progress'); INSERT INTO agent_runs(id,repository_id,wave,worker,model,issue_numbers,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,started_at,ended_at,harness,session_id) VALUES ('$AGENT_RUN','$REPO','W19k','fixture','fixture-model','[519]',11,2,3,5,1,'2026-11-14T00:00:00Z','2026-11-14T00:01:00Z','codex','mcp-519-session');" >/dev/null
TOKEN=$(curl -fsS -H 'Content-Type: application/json' -d "{\"email\":\"mcp-519@example.invalid\",\"password\":\"$PASSWORD\"}" "$API/api/v1/auth/login" | jq -r .token)
TOKEN=$(curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "{\"organization_id\":\"$ORG\"}" "$API/api/v1/auth/select-organization" | jq -r .token)
printf '# MCP 519 fixture\n\nThe fixture records a provider timeout.\n' >"$RUN/FIVE-MINUTES.md"

call() { local name=$1 args=$2 id=$3; printf '%s\n' "{\"jsonrpc\":\"2.0\",\"id\":$id,\"method\":\"tools/call\",\"params\":{\"name\":\"$name\",\"arguments\":$args}}"; }
invoke() { AGENTOPS_API_URL="$API" AGENTOPS_API_TOKEN="$TOKEN" AGENTOPS_FIVE_MINUTES_PATH="$RUN/FIVE-MINUTES.md" "$RUN/agentops" mcp; }
run_valid() {
  local out="$RUN/stdio.jsonl"
  { echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'; echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'; call list_issues "{\"repository_id\":\"$REPO\"}" 3; call wave_status "{\"wave_id\":\"$WAVE\"}" 4; call my_costs "{\"repository_id\":\"$REPO\",\"wave\":\"W19k\",\"issue_number\":519}" 5; call similar_failures "{\"query\":\"timeout\",\"repository_id\":\"$REPO\"}" 6; } | invoke >"$out" 2>"$RUN/stderr"
  jq -e 'select(.id==1 and .result.protocolVersion != null), select(.id==2 and (.result.tools|length)==4), select(.id==3 and (.result.content[0].text|contains("MCP verification issue"))), select(.id==4 and (.result.content[0].text|contains("W19k"))), select(.id==5 and (.result.content[0].text|contains("mcp-519-session"))), select(.id==6 and (.result.content[0].text|contains("provider timeout")))' "$out" >/dev/null
  rest=$(curl -fsS -H "Authorization: Bearer $TOKEN" "$API/api/v1/board?repository_id=$REPO")
  [[ "$rest" == *'MCP verification issue'* ]] || return 1
  curl -fsS -H "Authorization: Bearer $TOKEN" "$API/api/v1/waves/$WAVE" | jq -e '.data.name == "W19k"' >/dev/null
  curl -fsS -H "Authorization: Bearer $TOKEN" "$API/api/v1/agent-runs/aggregate?issue_number=519&repository_id=$REPO&wave=W19k" | jq -e 'length == 1' >/dev/null
  ! grep -R -F "$TOKEN" "$out" "$RUN/stderr" "$EVIDENCE" >/dev/null
  ! grep -R -F "$JWT_SECRET" "$out" "$RUN/stderr" "$EVIDENCE" >/dev/null
  ! grep -R -F "$PASSWORD" "$out" "$RUN/stderr" "$EVIDENCE" >/dev/null
}
run_valid
[[ $(call my_costs '{}' 90 | invoke | jq -r '.result.content[0].text') == *'mcp-519-session'* ]] || { echo 'empty my_costs arguments did not return authenticated aggregate' >&2; exit 1; }
[[ $(call similar_failures '{"query":"","repository_id":"'$REPO'"}' 91 | invoke | jq -r '.result.content[0].text') == *'[]'* ]] || { echo 'empty similar_failures query did not return an empty result' >&2; exit 1; }
for spec in 'list_issues {}' 'list_issues {"repository_id":7}' 'wave_status {}' 'wave_status {"wave_id":7}' 'my_costs {"issue_number":"bad"}' 'my_costs {"repository_id":7}' 'similar_failures {}' 'similar_failures {"query":7,"repository_id":"x"}'; do set -- $spec; if call "$1" "$2" 99 | invoke | jq -e '.error.code == -32001' >/dev/null; then :; else echo "invalid argument was accepted: $spec" >&2; exit 1; fi; done
cp "$RUN/stdio.jsonl" "$EVIDENCE/stdio.jsonl"; cp "$RUN/stderr" "$EVIDENCE/stderr.txt"
echo 'PASS: real MCP stdio initialization, tools/list, four tools, REST comparisons, valid/empty/invalid arguments, and secret redaction' | tee "$EVIDENCE/result.txt"
