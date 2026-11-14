#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${INGESTION_VERIFY_ARTIFACT_DIR:-"$ROOT/.agent-runs/W19-ingestion-514"}
RUN_DIR="$ROOT/.agent-runs/ingestion-verify-$$"
mkdir -p "$ARTIFACT_DIR" "$RUN_DIR" "$RUN_DIR/cache" "$RUN_DIR/modcache" "$RUN_DIR/buildx"
export GOCACHE="$RUN_DIR/cache" GOMODCACHE="$RUN_DIR/modcache" GOPATH="$RUN_DIR/gopath" GOTOOLCHAIN=local BUILDX_CONFIG="$RUN_DIR/buildx"
PROJECT="agent-ops-ingestion-$(id -u)-$$"
API_PORT=${INGESTION_VERIFY_API_PORT:-$((19800 + RANDOM % 700))}
PG_PORT=${INGESTION_VERIFY_PG_PORT:-$((16800 + RANDOM % 700))}
API="http://127.0.0.1:$API_PORT"; TOKEN=fixture-ingestion-token
REPO=00000000-0000-4000-8000-000000000514; KEY=01234567890123456789012345678901
compose=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" JWT_SECRET=ingestion-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" AGENT_RUN_TOKEN="$TOKEN" SCM_REPOSITORIES_JSON='[]' docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
log() { printf 'ingestion: %s\n' "$*" | tee -a "$ARTIFACT_DIR/commands.log"; }
fail() { log "FAIL: $*"; exit 1; }
sql() { "$ROOT/scripts/heavy.sh" "${compose[@]}" exec -T postgres psql -At -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "$1" | tr -d '\r'; }
cleanup() { local rc=$?; trap - EXIT INT TERM; "$ROOT/scripts/heavy.sh" "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || rc=1; [[ -z $("$ROOT/scripts/heavy.sh" docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' 2>/dev/null) ]] || rc=1; [[ -z $("$ROOT/scripts/heavy.sh" docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' 2>/dev/null) ]] || rc=1; [[ -z $("$ROOT/scripts/heavy.sh" docker network ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' 2>/dev/null) ]] || rc=1; cp "$RUN_DIR/commands.log" "$ARTIFACT_DIR/commands.log" 2>/dev/null || true; cp "$RUN_DIR/watch.log" "$ARTIFACT_DIR/watch.log" 2>/dev/null || true; rm -rf "$RUN_DIR"; [[ $rc -eq 0 ]] && log "cleanup verified: no owned containers, volumes, or networks ($PROJECT)"; exit "$rc"; }
trap cleanup EXIT INT TERM

"$ROOT/scripts/heavy.sh" "${compose[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null || fail 'API did not become ready'
log "PASS: fresh isolated stack ready project=$PROJECT api=$API"
"$ROOT/scripts/heavy.sh" "${compose[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "INSERT INTO repositories(id,provider,full_name,webhook_secret_env_ref) VALUES ('$REPO','github','fixture/ingestion-514','FIXTURE') ON CONFLICT (id) DO NOTHING; INSERT INTO pull_requests(id,repository_id,number,title,state,head_sha,branch,closes) VALUES ('00000000-0000-4000-8000-000000000515','$REPO',515,'fixture PR','open','sha-514','verify/w19f-ingestion-514','[514]') ON CONFLICT (repository_id,number) DO NOTHING;" >/dev/null
log 'PASS: SQL fixture persisted repository and branch-to-PR link'

cat >"$RUN_DIR/codex.jsonl" <<'EOF'
{"type":"thread.started","thread_id":"codex-514"}
{"type":"turn.completed","model":"fixture-codex","timestamp":"2026-11-14T01:00:00Z","gitBranch":"verify/w19f-ingestion-514","usage":{"input_tokens":101,"cached_input_tokens":11,"cache_write_input_tokens":3,"output_tokens":17,"reasoning_output_tokens":5}}
EOF
cat >"$RUN_DIR/opencode.jsonl" <<'EOF'
{"type":"step_finish","timestamp":1794618000000,"sessionID":"opencode-514","gitBranch":"verify/w19f-ingestion-514","part":{"tokens":{"input":103,"output":19,"reasoning":7,"cache":{"read":13,"write":4}}}}
EOF
cat >"$RUN_DIR/claude.jsonl" <<'EOF'
{"type":"assistant","uuid":"claude-514-1","sessionId":"claude-514","model":"fixture-claude","timestamp":"2026-11-14T01:00:00Z","gitBranch":"verify/w19f-ingestion-514","message":{"model":"fixture-claude","usage":{"input_tokens":107,"cache_read_input_tokens":17,"cache_creation_input_tokens":5,"output_tokens":23,"reasoning_tokens":9}}}
EOF
cli="$RUN_DIR/agentops"; cli_source="$RUN_DIR/cli-source"; cp -R "$ROOT/api/." "$cli_source"; perl -0pi -e 's/^go 1\.26\.11$/go 1.26.2/m' "$cli_source/go.mod"; (cd "$cli_source" && GOTOOLCHAIN=local go build -o "$cli" ./cmd/agentops)
push() { local harness=$1 file=$2 issues=$3; log "CLI agentops push --harness $harness --repository-id $REPO --wave W19 --worker fixture --branch verify/w19f-ingestion-514 --issue-numbers '$issues' $file"; AGENT_RUN_TOKEN="$TOKEN" AGENT_RUN_MODEL=openai/gpt-fixture "$cli" push --harness "$harness" --endpoint "$API/api/v1/agent-runs" --repository-id "$REPO" --wave W19 --worker fixture --branch verify/w19f-ingestion-514 --issue-numbers "$issues" "$file"; }
push codex "$RUN_DIR/codex.jsonl" '[514]'; push opencode "$RUN_DIR/opencode.jsonl" '[]'; push claude "$RUN_DIR/claude.jsonl" '[514]'
row() { sql "SELECT count(*)||'|'||coalesce(min(input_tokens),0)||'|'||coalesce(min(cached_input_tokens),0)||'|'||coalesce(min(cache_write_tokens),0)||'|'||coalesce(min(output_tokens),0)||'|'||coalesce(min(reasoning_tokens),0)||'|'||coalesce(min(model),'')||'|'||coalesce(min(issue_numbers::text),'')||'|'||coalesce(min(pr_number),0)||'|'||coalesce(min(branch),'') FROM agent_runs WHERE repository_id='$REPO' AND harness='$1'"; }
check() { [[ "$1" == "$2" ]] || fail "$3 expected=$1 actual=$2"; log "PASS: $3 expected=$1 actual=$2"; }
check '1|101|11|3|17|5|fixture-codex|[514]|515|verify/w19f-ingestion-514' "$(row codex)" 'Codex token categories, issue numbers, branch-to-PR link'
check '1|103|13|4|19|7|openai/gpt-fixture|[]|515|verify/w19f-ingestion-514' "$(row opencode)" 'OpenCode token categories, empty issue_numbers, branch-to-PR link'
check '1|107|17|5|23|9|fixture-claude|[514]|515|verify/w19f-ingestion-514' "$(row claude)" 'Claude token categories, issue numbers, branch-to-PR link'
push codex "$RUN_DIR/codex.jsonl" '[514]'; check '1' "$(sql "SELECT count(*) FROM agent_runs WHERE repository_id='$REPO' AND harness='codex' AND session_id='codex-514'")" 'idempotent Codex re-push preserves identity cardinality'

watch_root="$RUN_DIR/watch"; mkdir -p "$watch_root/project"
printf '%s\n' '{"type":"assistant","uuid":"watch-514-1","sessionId":"watch-514","model":"fixture-watch","timestamp":"2026-11-14T02:00:00Z","gitBranch":"verify/w19f-ingestion-514","message":{"model":"fixture-watch","usage":{"input_tokens":109,"cache_read_input_tokens":19,"cache_creation_input_tokens":6,"output_tokens":29,"reasoning_tokens":10}}}' >"$watch_root/project/live.jsonl"
printf '%s' '{"type":"assistant","uuid":"watch-514-2","sessionId":"watch-514"' >>"$watch_root/project/live.jsonl"
watch_cp="$RUN_DIR/checkpoint.json"; AGENT_RUN_TOKEN="$TOKEN" "$cli" watch --root "$watch_root" --checkpoint "$watch_cp" --endpoint "$API/api/v1/agent-runs" --repository-id "$REPO" --wave W19 --worker watcher --branch verify/w19f-ingestion-514 --interval 100ms >"$RUN_DIR/watch.log" 2>&1 & watch_pid=$!
for _ in $(seq 1 20); do [[ "$(sql "SELECT count(*) FROM agent_runs WHERE repository_id='$REPO' AND harness='claude' AND session_id='watch-514'")" == 1 ]] && break; sleep .2; done
kill -TERM "$watch_pid" 2>/dev/null || true; wait "$watch_pid" || true
check '1' "$(sql "SELECT count(*) FROM agent_runs WHERE repository_id='$REPO' AND harness='claude' AND session_id='watch-514'")" 'watch ingests complete lines while deferring partial trailing line'
cat >"$watch_root/project/live.jsonl" <<'EOF'
{"type":"assistant","uuid":"watch-514-1","sessionId":"watch-514","model":"fixture-watch","timestamp":"2026-11-14T02:00:00Z","gitBranch":"verify/w19f-ingestion-514","message":{"model":"fixture-watch","usage":{"input_tokens":109,"cache_read_input_tokens":19,"cache_creation_input_tokens":6,"output_tokens":29,"reasoning_tokens":10}}}
{"type":"assistant","uuid":"watch-514-2","sessionId":"watch-514","model":"fixture-watch","timestamp":"2026-11-14T02:01:00Z","gitBranch":"verify/w19f-ingestion-514","message":{"model":"fixture-watch","usage":{"input_tokens":111,"cache_read_input_tokens":21,"cache_creation_input_tokens":7,"output_tokens":31,"reasoning_tokens":12}}}
EOF
AGENT_RUN_TOKEN="$TOKEN" timeout 3 "$cli" watch --root "$watch_root" --checkpoint "$watch_cp" --endpoint "$API/api/v1/agent-runs" --repository-id "$REPO" --wave W19 --worker watcher --branch verify/w19f-ingestion-514 --interval 100ms >/dev/null 2>&1 || true
watch_row() { sql "SELECT count(*)||'|'||coalesce(min(input_tokens),0)||'|'||coalesce(min(cached_input_tokens),0)||'|'||coalesce(min(cache_write_tokens),0)||'|'||coalesce(min(output_tokens),0)||'|'||coalesce(min(reasoning_tokens),0)||'|'||coalesce(min(model),'')||'|'||coalesce(min(issue_numbers::text),'')||'|'||coalesce(min(pr_number),0)||'|'||coalesce(min(branch),'') FROM agent_runs WHERE repository_id='$REPO' AND harness='claude' AND session_id='watch-514'"; }
check '1|220|40|13|60|22|fixture-watch|[]|515|verify/w19f-ingestion-514' "$(watch_row)" 'watch checkpoint restart updates cumulative persisted Claude values'
check '1' "$(jq '.files | length' "$watch_cp")" 'watch checkpoint persisted after restart'
log 'PASS: measured watch contract is Claude JSONL only; Codex/OpenCode watch is unsupported by --root contract'

set +e; wrong=$(AGENT_RUN_TOKEN=wrong-token "$cli" push --harness codex --endpoint "$API/api/v1/agent-runs" --repository-id "$REPO" --wave W19 --worker fixture "$RUN_DIR/codex.jsonl" 2>&1); wrong_rc=$?; missing=$(env -u AGENT_RUN_TOKEN "$cli" push --harness codex --endpoint "$API/api/v1/agent-runs" --repository-id "$REPO" --wave W19 --worker fixture "$RUN_DIR/codex.jsonl" 2>&1); missing_rc=$?; set -e
[[ $wrong_rc -ne 0 && "$wrong" == *'HTTP 401'* ]] || fail "wrong token expected HTTP 401 actual rc=$wrong_rc output=$wrong"; [[ $missing_rc -ne 0 && "$missing" == *'AGENT_RUN_TOKEN is required'* ]] || fail "missing token expected CLI guard actual rc=$missing_rc output=$missing"; log "PASS: wrong token rejected through HTTP (rc=$wrong_rc); missing token rejected by CLI guard (rc=$missing_rc)"

control="$RUN_DIR/control"; cp -R "$cli_source" "$control"; python3 - "$control/cmd/agentops/main.go" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text(); old='"session_id": session.ID'
if old not in s: raise SystemExit('control target missing')
p.write_text(s.replace(old, '"session_id": "mutated-session"', 1))
PY
(cd "$control" && GOTOOLCHAIN=local go build -o "$RUN_DIR/agentops-mutated" ./cmd/agentops)
set +e; AGENT_RUN_TOKEN="$TOKEN" "$RUN_DIR/agentops-mutated" push --harness codex --endpoint "$API/api/v1/agent-runs" --repository-id "$REPO" --wave W19 --worker fixture --branch verify/w19f-ingestion-514 --issue-numbers '[514]' "$RUN_DIR/codex.jsonl" >/dev/null 2>&1; control_rc=$?; set -e
[[ $control_rc -eq 0 ]] || fail "negative control mutated CLI failed before assertion rc=$control_rc"; control_rows=$(sql "SELECT count(*) FROM agent_runs WHERE repository_id='$REPO' AND harness='codex'"); [[ "$control_rows" == 2 ]] || fail "negative control expected duplicate identity row count=2 actual=$control_rows"; log "PASS: deliberate session-identity guard mutation failed intended cardinality assertion with identical Codex input (rows=$control_rows)"
log "PASS: all #514 acceptance checks completed source=$(git -C "$ROOT" rev-parse HEAD)"
