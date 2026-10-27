#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-run-acceptance-$(id -u)-$$"
API_PORT=${AGENT_RUN_ACCEPTANCE_API_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
PG_PORT=${AGENT_RUN_ACCEPTANCE_PG_PORT:-$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')}
TOKEN=acceptance-secret REPO=a1000000-0000-4000-8000-000000000001 REPO2=a2000000-0000-4000-8000-000000000002
export JWT_SECRET=${JWT_SECRET:-acceptance-jwt-secret} REPOSITORY_ENCRYPTION_KEY=${REPOSITORY_ENCRYPTION_KEY:-01234567890123456789012345678901}
API="http://127.0.0.1:$API_PORT" DB="postgres://agent_ops:agent_ops_dev@127.0.0.1:$PG_PORT/agent_ops?sslmode=disable"
API_PID= TMP=
die() { echo "acceptance: FAIL: $*" >&2; exit 1; }
log() { echo "acceptance: $*"; }
cleanup() {
 local status=$? listener containers volumes
 trap - EXIT INT TERM
 if [[ -n "$API_PID" ]]; then kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; fi
 listener=$(lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null || true)
 if [[ -n "$listener" ]]; then log "cleanup FAILED: API listener remains on $API_PORT: $listener"; status=1; else log "cleanup verified: no API listener on $API_PORT"; fi
 if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then log "cleanup FAILED: API PID $API_PID remains"; status=1; else log "cleanup verified: API PID absent"; fi
  if [[ ${AGENT_RUN_ACCEPTANCE_SKIP_COMPOSE_CLEANUP:-0} != 1 ]]; then
    docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null || { log 'cleanup FAILED: compose down'; status=1; }
  fi
  containers=$(docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' || true)
  volumes=$(docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' || true)
  [[ -z "$containers" ]] || { log "cleanup FAILED: owned containers remain: $containers"; status=1; }
  [[ -z "$volumes" ]] || { log "cleanup FAILED: owned volumes remain: $volumes"; status=1; }
  if [[ ${AGENT_RUN_ACCEPTANCE_SKIP_COMPOSE_CLEANUP:-0} == 1 ]]; then
    log "cleanup removal regression: deliberately retained fixtures for $PROJECT"
    if [[ ${AGENT_RUN_ACCEPTANCE_FORCE_CLEANUP:-0} == 1 ]]; then
      docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null || { log "cleanup FAILED: outer fixture cleanup for $PROJECT"; status=1; }
      containers=$(docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' || true)
      volumes=$(docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' || true)
    fi
  fi
  if [[ -n "$containers" || -n "$volumes" ]]; then
    log "cleanup FAILED: owned containers/volumes remain ($PROJECT)"
    status=1
  else
    log "cleanup verified: no owned containers or volumes ($PROJECT)"
  fi
  [[ -z "$TMP" ]] || rm -rf "$TMP" || { log 'cleanup FAILED: temporary directory'; status=1; }
  if [[ ${AGENT_RUN_ACCEPTANCE_INJECT_CLEANUP_FAILURE:-0} == 1 && ${AGENT_RUN_ACCEPTANCE_DISABLE_CLEANUP_INJECTION:-0} != 1 ]]; then log 'cleanup FAILED: injected cleanup failure'; status=1; fi
  if [[ ${AGENT_RUN_ACCEPTANCE_EXTRA_CLEANUP_FAILURE:-0} == 1 ]]; then log 'cleanup FAILED: injected additional cleanup failure'; status=1; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
[[ "$PG_PORT" != 5432 ]] || die 'refusing PostgreSQL host port 5432'
[[ -z "$(lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null || true)" ]] || die "API port $API_PORT busy"
[[ -z "$(lsof -nP -iTCP:"$PG_PORT" -sTCP:LISTEN 2>/dev/null || true)" ]] || die "PostgreSQL port $PG_PORT busy"
TMP=$(mktemp -d); BIN="$TMP/api"
(cd "$ROOT/api" && GOCACHE="$TMP/go-cache" go build -o "$BIN" .)
POSTGRES_HOST_PORT="$PG_PORT" POSTGRES_PUBLISH=127.0.0.1 docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" up -d postgres >/dev/null
postgres_ready=0
for _ in $(seq 1 60); do
  if docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1; then postgres_ready=1; break; fi
  sleep 1
done
(( postgres_ready )) || { docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" logs postgres >&2 || true; die 'PostgreSQL not ready'; }
export AGENT_RUN_TOKEN="$TOKEN" AGENT_RUN_MODEL_PRICES='{"gpt-accept":{"input_per_million":1,"cached_input_per_million":2,"cache_write_per_million":3,"output_per_million":4,"reasoning_per_million":5}}' DATABASE_URL="$DB" HTTP_ADDR=":$API_PORT" JWT_SECRET='acceptance-jwt-secret' REPOSITORY_ENCRYPTION_KEY='01234567890123456789012345678901' SCM_REPOSITORIES_JSON='[]'
"$BIN" >"$TMP/api.log" 2>&1 & API_PID=$!
api_ready=0
for _ in $(seq 1 60); do
  if curl -fsS "$API/readyz" >/dev/null 2>&1; then api_ready=1; break; fi
  sleep 1
done
(( api_ready )) || { cat "$TMP/api.log" >&2; die 'API readiness timeout'; }
curl -fsS "$API/readyz" >/dev/null || { cat "$TMP/api.log" >&2; die 'API not ready'; }
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops -c "INSERT INTO repositories (id,provider,full_name,webhook_secret_env_ref) VALUES ('$REPO','github','acceptance/one','ACCEPTANCE_SECRET'),('$REPO2','github','acceptance/two','ACCEPTANCE_SECRET') ON CONFLICT (id) DO NOTHING" >/dev/null
post() { curl -sS -w '\n%{http_code}' -H 'Content-Type: application/json' -H "X-Agent-Run-Token: $TOKEN" "$API/api/v1/agent-runs" -d "$1"; }
expect_code() { [[ "${1##*$'\n'}" == "$2" ]] || die "$3: expected HTTP $2"; }
base() { printf '{"repository_id":"%s","wave":"%s","worker":"%s","model":"gpt-accept","issue_numbers":[%s],"pr_number":3191,"input_tokens":100,"cached_input_tokens":20,"cache_write_tokens":30,"output_tokens":40,"reasoning_tokens":50,"started_at":"2026-10-24T09:00:00Z","ended_at":"2026-10-24T09:05:00Z"}' "$1" "$2" "$3" "$4"; }
identity_base() { printf '{"repository_id":"%s","wave":"W9D","worker":"identity","model":"gpt-accept","issue_numbers":[60],"input_tokens":%s,"cached_input_tokens":2,"cache_write_tokens":3,"output_tokens":4,"reasoning_tokens":5,"started_at":"2026-10-24T10:00:00Z","ended_at":"2026-10-24T10:05:00Z","harness":"%s","session_id":"%s"}' "$1" "$2" "$3" "$4"; }
one=$(post "$(base "$REPO" W3G wire '191,192')"); expect_code "$one" 201 'authenticated POST'; id1=$(jq -r .data.id <<<"${one%$'\n'*}")
two=$(post "$(base "$REPO" W3G-distractor wave '191')"); expect_code "$two" 201 'wave distractor POST'; id2=$(jq -r .data.id <<<"${two%$'\n'*}")
three=$(post "$(base "$REPO2" W3G repo '191')"); expect_code "$three" 201 'repository distractor POST'; id3=$(jq -r .data.id <<<"${three%$'\n'*}")
four=$(post "$(base "$REPO" W3G issue '999')"); expect_code "$four" 201 'issue distractor POST'; id4=$(jq -r .data.id <<<"${four%$'\n'*}")
negative_body='{"repository_id":"'"$REPO"'","wave":"W3G","worker":"bad","model":"gpt-accept","issue_numbers":[191],"pr_number":3191,"input_tokens":-1,"cached_input_tokens":0,"cache_write_tokens":0,"output_tokens":0,"reasoning_tokens":0,"started_at":"2026-10-24T09:00:00Z","ended_at":"2026-10-24T09:05:00Z"}'
negative=$(post "$negative_body"); [[ "${negative##*$'\n'}" == 400 ]] || die 'negative token validation: expected HTTP 400'
unauthorized=$(curl -sS -w '\n%{http_code}' -H 'Content-Type: application/json' "$API/api/v1/agent-runs" -d "$(base "$REPO" W3G bad '191')"); expect_code "$unauthorized" 401 'missing authentication'
identity=$(post "$(identity_base "$REPO" 111 codex session-1)"); expect_code "$identity" 201 'identity initial POST'
identity_id=$(jq -r .data.id <<<"${identity%$'\n'*}")
duplicate=$(post "$(identity_base "$REPO" 111 codex session-1)"); expect_code "$duplicate" 201 'identity duplicate POST'
growth=$(post "$(identity_base "$REPO" 222 codex session-1)"); expect_code "$growth" 201 'identity cumulative update'
isolated=$(post "$(identity_base "$REPO" 333 opencode session-1)"); expect_code "$isolated" 201 'harness isolation POST'
concurrent_pids=()
for _ in 1 2 3 4; do
  (post "$(identity_base "$REPO" 444 claude session-concurrent)" >/dev/null) & concurrent_pids+=("$!")
done
for pid in "${concurrent_pids[@]}"; do wait "$pid" || die "concurrent duplicate request $pid failed"; done
identity_snapshot=$(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT count(*), min(input_tokens), max(input_tokens) FROM agent_runs WHERE repository_id='$REPO' AND session_id='session-1' AND harness='codex'")
[[ "$identity_snapshot" == '1|222|222' ]] || die "IDENTITY UPSERT ASSERTION: expected one cumulative codex row at 222 tokens got $identity_snapshot"
concurrent_snapshot=$(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT count(*), max(input_tokens) FROM agent_runs WHERE repository_id='$REPO' AND session_id='session-concurrent' AND harness='claude'")
[[ "$concurrent_snapshot" == '1|444' ]] || die "CONCURRENT UPSERT ASSERTION: expected one claude row at 444 tokens got $concurrent_snapshot"
kill "$API_PID"; wait "$API_PID" 2>/dev/null || true; API_PID=
"$BIN" >"$TMP/api-restart.log" 2>&1 & API_PID=$!
api_ready=0
for _ in $(seq 1 60); do
  if curl -fsS "$API/readyz" >/dev/null 2>&1; then api_ready=1; break; fi
  sleep 1
done
(( api_ready )) || { cat "$TMP/api-restart.log" >&2; die 'API restart readiness timeout'; }
curl -fsS "$API/readyz" >/dev/null || { cat "$TMP/api-restart.log" >&2; die 'API restart not ready'; }
persistent_snapshot=$(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT count(*), min(input_tokens), max(input_tokens) FROM agent_runs WHERE id='$identity_id'")
[[ "$persistent_snapshot" == '1|222|222' ]] || die "PERSISTENCE ASSERTION: expected restarted row 1|222|222 got $persistent_snapshot"
row=$(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT repository_id,wave,worker,model,issue_numbers::text,pr_number,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,started_at AT TIME ZONE 'UTC',ended_at AT TIME ZONE 'UTC' FROM agent_runs WHERE id='$id1'")
expected_row="$REPO|W3G|wire|gpt-accept|[191, 192]|3191|100|20|30|40|50|2026-10-24 09:00:00|2026-10-24 09:05:00"
if [[ ${AGENT_RUN_ACCEPTANCE_CORRUPT_TIMESTAMP:-0} == 1 ]]; then
 docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops -c "UPDATE agent_runs SET started_at=started_at + interval '1 minute' WHERE id='$id1'" >/dev/null
 row=$(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT repository_id,wave,worker,model,issue_numbers::text,pr_number,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,started_at AT TIME ZONE 'UTC',ended_at AT TIME ZONE 'UTC' FROM agent_runs WHERE id='$id1'")
fi
[[ "$row" == "$expected_row" ]] || die "exact SQL normalization/timestamp mismatch: $row"
expected_exact=$(jq -cn '[{wave:"W3G",worker:"wire",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"wire",issue_number:192,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064}]|sort_by(.wave,.worker,.issue_number)')
expected_repo_wave=$(jq -cn '[{wave:"W3G",worker:"wire",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"wire",issue_number:192,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"issue",issue_number:999,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064}]|sort_by(.wave,.worker,.issue_number)')
expected_repo_issue=$(jq -cn '[{wave:"W3G",worker:"wire",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"wire",issue_number:192,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G-distractor",worker:"wave",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064}]|sort_by(.wave,.worker,.issue_number)')
expected_wave_issue=$(jq -cn '[{wave:"W3G",worker:"wire",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"wire",issue_number:192,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"repo",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064}]|sort_by(.wave,.worker,.issue_number)')
expected_repo=$(jq -cn '[{wave:"W3G",worker:"wire",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"wire",issue_number:192,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G-distractor",worker:"wave",issue_number:191,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064},{wave:"W3G",worker:"issue",issue_number:999,runs:1,input_tokens:100,cached_input_tokens:20,cache_write_tokens:30,output_tokens:40,reasoning_tokens:50,cost_usd:0.00064}]|sort_by(.wave,.worker,.issue_number)')
expected_all="$expected_repo"
list_assert() {
 local url=$1 expected_run_ids=$2 expected_ids actual_ids list
 [[ ${AGENT_RUN_ACCEPTANCE_SKIP_LIST:-0} == 1 ]] && return 0
 list=$(curl -fsS "$url")
 expected_ids=$(tr ',' '\n' <<<"$expected_run_ids" | sort | paste -sd, -)
 actual_ids=$(jq -r '[.data[].id]|sort|join(",")' <<<"$list")
  [[ "$actual_ids" == "$expected_ids" ]] || die "LIST FILTER ASSERTION: expected run ids $expected_ids got $actual_ids"
}
agg_assert() {
 local url=$1 expected=$2 actual agg
 agg=$(curl -fsS "$url")
 actual=$(jq -c '[.data[]]|sort_by(.wave,.worker,.issue_number)' <<<"$agg")
 [[ "$actual" == "$expected" ]] || die "AGGREGATE FILTER ASSERTION: expected complete snapshot $expected got $actual"
}
list_assert "$API/api/v1/agent-runs?repository_id=$REPO&wave=W3G&issue_number=191" "$id1"
agg_assert "$API/api/v1/agent-runs/aggregate?repository_id=$REPO&wave=W3G&issue_number=191" "$expected_exact"
list_assert "$API/api/v1/agent-runs?repository_id=$REPO&wave=W3G" "$id1,$id4"
agg_assert "$API/api/v1/agent-runs/aggregate?repository_id=$REPO&wave=W3G" "$expected_repo_wave"
list_assert "$API/api/v1/agent-runs?repository_id=$REPO&issue_number=191" "$id1,$id2"
agg_assert "$API/api/v1/agent-runs/aggregate?repository_id=$REPO&issue_number=191" "$expected_repo_issue"
list_assert "$API/api/v1/agent-runs?wave=W3G&issue_number=191" "$id1,$id3"
agg_assert "$API/api/v1/agent-runs/aggregate?wave=W3G&issue_number=191" "$expected_wave_issue"
log "wire/SQL acceptance passed: ids=$id1; exact aggregate snapshot=$expected_exact"
log 'success path complete'
