#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
EVIDENCE=${MCP_EVIDENCE_DIR:-"$ROOT/.agent-runs/W15e-opencode-mcp-integration"}
PROJECT="agentops-mcp-$(id -u)-$$"
API_PORT=${MCP_API_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
PG_PORT=${MCP_PG_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
API="http://127.0.0.1:$API_PORT"; DB="postgres://agent_ops:agent_ops_dev@127.0.0.1:$PG_PORT/agent_ops?sslmode=disable"
JWT_SECRET=mcp-disposable-jwt; API_PID=
export JWT_SECRET REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901
REPO=15000000-0000-4000-8000-000000000001; ORG=15000000-0000-4000-8000-000000000002; ACCOUNT=15000000-0000-4000-8000-000000000003; WAVE=15000000-0000-4000-8000-000000000004; ISSUE=15000000-0000-4000-8000-000000000005; RUN=15000000-0000-4000-8000-000000000006
mkdir -p "$EVIDENCE"; git -C "$ROOT" rev-parse HEAD >"$EVIDENCE/source-sha.txt"
cleanup() { local rc=$? c v; trap - EXIT INT TERM; [[ -z "$API_PID" ]] || { kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; }; POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null 2>&1 || rc=1; c=$(docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}'); v=$(docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}'); [[ -z "$c$v" ]] || rc=1; printf 'cleanup verified: containers=%s volumes=%s\n' "${c:-none}" "${v:-none}" >"$EVIDENCE/cleanup.txt"; rm -f "$EVIDENCE/.jwt"; exit "$rc"; }
trap cleanup EXIT INT TERM
[[ "$API_PORT" != 18080 && "$PG_PORT" != 5432 ]] || { echo 'refusing default ports' >&2; exit 1; }
POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" up -d postgres >/dev/null
for _ in $(seq 1 60); do docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null
GOCACHE=${GOCACHE:-/private/tmp/w14c-gocache} GOMODCACHE=${GOMODCACHE:-/private/tmp/w14c-gomodcache} GOPATH=${GOPATH:-/private/tmp/w14c-gopath} go build -o "$EVIDENCE/agentops" "$ROOT/api" 2>/dev/null || (cd "$ROOT/api" && GOCACHE=${GOCACHE:-/private/tmp/w14c-gocache} GOMODCACHE=${GOMODCACHE:-/private/tmp/w14c-gomodcache} GOPATH=${GOPATH:-/private/tmp/w14c-gopath} go build -o "$EVIDENCE/agentops" .)
DATABASE_URL="$DB" HTTP_ADDR=":$API_PORT" JWT_SECRET="$JWT_SECRET" REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 SCM_REPOSITORIES_JSON='[]' "$EVIDENCE/agentops" >"$EVIDENCE/api.log" 2>&1 & API_PID=$!
for _ in $(seq 1 60); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done; curl -fsS "$API/readyz" >/dev/null
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops <<SQL >/dev/null
INSERT INTO accounts(id,email,password_hash) VALUES ('$ACCOUNT','mcp@example.invalid','unused') ON CONFLICT DO NOTHING;
INSERT INTO organizations(id,name) VALUES ('$ORG','MCP disposable organization') ON CONFLICT DO NOTHING;
INSERT INTO organization_memberships(id,organization_id,account_id,role) VALUES ('15000000-0000-4000-8000-000000000007','$ORG','$ACCOUNT','owner') ON CONFLICT DO NOTHING;
INSERT INTO repositories(id,provider,full_name,webhook_secret_env_ref,organization_id) VALUES ('$REPO','github','fixture/mcp','MCP_SECRET','$ORG') ON CONFLICT DO NOTHING;
INSERT INTO issues(id,repository_id,number,title,state,status,labels,assignee) VALUES ('$ISSUE','$REPO',8501,'MCP seeded issue','open','review','["status:review","area:docs"]','fixture-worker') ON CONFLICT DO NOTHING;
INSERT INTO waves(id,organization_id,name,brief,state) VALUES ('$WAVE','$ORG','W15e','OpenCode MCP proof','in_progress') ON CONFLICT DO NOTHING;
INSERT INTO agent_runs(id,repository_id,wave,worker,model,issue_numbers,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,started_at,ended_at,harness,session_id) VALUES ('$RUN','$REPO','W15e','fixture-worker','fixture-model','[85]',11,2,3,5,1,'2026-10-30T00:00:00Z','2026-10-30T00:05:00Z','opencode','mcp-seeded-session') ON CONFLICT DO NOTHING;
SQL
JWT_HEADER=$(printf '%s' '{"alg":"HS256","typ":"JWT"}' | openssl base64 -A | tr '+/' '-_' | tr -d '='); JWT_PAYLOAD=$(printf '{"sub":"%s","email":"mcp@example.invalid","organization_id":"%s","iat":1793318400,"exp":1793322000}' "$ACCOUNT" "$ORG" | openssl base64 -A | tr '+/' '-_' | tr -d '='); JWT_SIG=$(printf '%s.%s' "$JWT_HEADER" "$JWT_PAYLOAD" | openssl dgst -sha256 -hmac "$JWT_SECRET" -binary | openssl base64 -A | tr '+/' '-_' | tr -d '='); printf '%s.%s.%s' "$JWT_HEADER" "$JWT_PAYLOAD" "$JWT_SIG" >"$EVIDENCE/.jwt"; chmod 600 "$EVIDENCE/.jwt"
printf '# MCP seeded failure\n\nThe fixture records a provider timeout for the disposable OpenCode proof.\n' >"$EVIDENCE/FIVE-MINUTES.md"
cat >"$EVIDENCE/opencode.json" <<JSON
{"\$schema":"https://opencode.ai/config.json","mcp":{"agentops":{"type":"local","command":["bash","-lc","cd $ROOT/api && go run ./cmd/agentops mcp"],"environment":{"AGENTOPS_API_URL":"$API","AGENTOPS_API_TOKEN":"$(cat "$EVIDENCE/.jwt")","AGENTOPS_FIVE_MINUTES_PATH":"$EVIDENCE/FIVE-MINUTES.md"}}}}
JSON
(cd "$EVIDENCE" && opencode run --model openai/gpt-6-luna --variant low --format json --auto 'Use the agentops MCP server only. Call tools/list, then list_issues for repository 15000000-0000-4000-8000-000000000001, wave_status for wave 15000000-0000-4000-8000-000000000004, my_costs for repository 15000000-0000-4000-8000-000000000001 and wave W15e, and similar_failures with query timeout and that repository. Report each tool name and the seeded value observed. Do not use shell or HTTP.') >"$EVIDENCE/session.jsonl" 2>"$EVIDENCE/opencode.log"
[[ -s "$EVIDENCE/session.jsonl" ]] || { printf 'OpenCode did not emit a session; diagnostic: %s\n' "$(tr '\n' ' ' <"$EVIDENCE/opencode.log")" >"$EVIDENCE/protocol-diagnostic.txt"; exit 1; }
grep -q 'MCP seeded issue' "$EVIDENCE/session.jsonl"; grep -q 'W15e' "$EVIDENCE/session.jsonl"; grep -q 'mcp-seeded-session' "$EVIDENCE/session.jsonl"; grep -qi 'provider timeout' "$EVIDENCE/session.jsonl"
if grep -q 'MCP definitely absent issue' "$EVIDENCE/session.jsonl"; then echo 'negative control unexpectedly passed' >"$EVIDENCE/control.txt"; exit 1; fi
echo 'failing control: absent seeded result rejected (grep exit 1); restored assertion passed' >"$EVIDENCE/control.txt"
printf 'OpenCode exit status: 0\nTools: list_issues, wave_status, my_costs, similar_failures\nsource SHA: %s\n' "$(cat "$EVIDENCE/source-sha.txt")" >"$EVIDENCE/summary.txt"

