#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CLIENT=${CLIENT_DIR:-"$ROOT/.agent-runs/w7r-smoke-client"}
PROJECT="agent-ops-smoke-$(id -u)-$$"
API_PORT=${SMOKE_API_PORT:-18080}
CLIENT_PORT=${SMOKE_CLIENT_PORT:-4173}
POSTGRES_HOST_PORT=${POSTGRES_HOST_PORT:-5432}
API_URL="http://127.0.0.1:$API_PORT"
CLIENT_URL="http://127.0.0.1:$CLIENT_PORT"
PREVIEW_PID=
ARTIFACT_DIR=${SMOKE_ARTIFACT_DIR:-"$ROOT/.agent-runs/smoke"}
COMPOSE_ENV=(env "POSTGRES_PUBLISH=${POSTGRES_PUBLISH-127.0.0.1}" "POSTGRES_HOST_PORT=$POSTGRES_HOST_PORT" "SMOKE_API_PORT=$API_PORT" "SMOKE_CLIENT_PORT=$CLIENT_PORT" "CLIENT_DIR=$CLIENT" "JWT_SECRET=${JWT_SECRET:?set a disposable JWT_SECRET for smoke}" "REPOSITORY_ENCRYPTION_KEY=${REPOSITORY_ENCRYPTION_KEY:?set a disposable 32-byte REPOSITORY_ENCRYPTION_KEY for smoke}")
export SMOKE_GITLAB_FIXTURE_URL=${SMOKE_GITLAB_FIXTURE_URL:-http://host.docker.internal:${SMOKE_FIXTURE_PORT:-18081}}

compose() { "${COMPOSE_ENV[@]}" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" "$@"; }

log() { printf 'smoke: %s\n' "$*"; }
FIXTURE_PID=
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ -n "$PREVIEW_PID" ]]; then
    preview_tree=$(python3 -c 'import subprocess,sys,time; root=int(sys.argv[1]); seen=set(); stack=[root];
while stack:
 p=stack.pop();
 if p in seen: continue
 seen.add(p)
 out=subprocess.run(["pgrep","-P",str(p)],capture_output=True,text=True); stack.extend(int(x) for x in out.stdout.split())
time.sleep(1)
print(" ".join(map(str, seen)))' "$PREVIEW_PID" 2>/dev/null || printf '%s' "$PREVIEW_PID")
    for child in $preview_tree; do kill -TERM "$child" 2>/dev/null || true; done
    for _ in $(seq 1 30); do
      alive=0
      for child in $preview_tree; do kill -0 "$child" 2>/dev/null && alive=1; done
      [[ $alive -eq 0 ]] && break
      sleep 0.1
    done
    # The preview wrapper can exit before its descendants have released the
    # listening socket. Wait for the captured process tree, not just root.
    for _ in $(seq 1 30); do
      kill -0 "$PREVIEW_PID" 2>/dev/null || break
      sleep 0.1
    done
    if kill -0 "$PREVIEW_PID" 2>/dev/null; then kill -KILL "$PREVIEW_PID" 2>/dev/null || true; fi
    wait "$PREVIEW_PID" 2>/dev/null || true
    listener=
    for _ in $(seq 1 100); do
      listener=$(lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null || true)
      [[ -n "$listener" ]] || break
      sleep 0.1
    done
  fi
  if [[ -n "$PREVIEW_PID" ]] && [[ -n "${listener:-}" ]]; then log "cleanup failed: preview listener remains on $CLIENT_PORT"; status=1; fi
  if [[ $status -ne 0 ]]; then compose logs --no-color >"$ARTIFACT_DIR/compose.log" 2>/dev/null || true; fi
  if [[ -n "$FIXTURE_PID" ]]; then kill "$FIXTURE_PID" 2>/dev/null || true; wait "$FIXTURE_PID" 2>/dev/null || true; fi
  if [[ -n "${FIXTURE_PORT:-}" ]] && lsof -nP -iTCP:"$FIXTURE_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then log "cleanup failed: fixture listener remains on $FIXTURE_PORT"; status=1; fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then log "cleanup failed: containers remain for $PROJECT"; status=1; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then log "cleanup failed: volumes remain for $PROJECT"; status=1; fi
  if [[ $status -eq 0 ]]; then log "cleanup verified: no owned containers or volumes ($PROJECT)"; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$ARTIFACT_DIR"
if [[ ! -d "$CLIENT/.git" ]]; then
  log "client checkout missing at $CLIENT; clone canonical repository"
  mkdir -p "$(dirname "$CLIENT")"
  git clone https://github.com/0Hoag/agent-ops-client.git "$CLIENT"
else
  [[ -z "$(git -C "$CLIENT" status --porcelain --untracked-files=all | grep -vE '^\?\? (test-results/|dist/)' || true)" ]] || { log "client checkout has user changes; refusing to update"; exit 1; }
  git -C "$CLIENT" fetch origin main
  git -C "$CLIENT" merge --ff-only origin/main
fi
log "client checkout $(git -C "$CLIENT" rev-parse HEAD)"
if lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null | grep -q . || lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then
  log "smoke port already has a listener (API $API_PORT, client $CLIENT_PORT); set SMOKE_*_PORT to unused ports"
  exit 1
fi

compose up -d --build postgres api
services=$(compose ps --services --filter status=running | sort)
if [[ "$services" != $'api\npostgres' ]]; then
  log "unexpected Compose services: ${services//$'\n'/, }"
  exit 1
fi
if compose ps -q client | grep -q .; then
  log "unexpected Compose client container/listener"
  exit 1
fi
for _ in $(seq 1 90); do
  if curl --fail --silent "$API_URL/readyz" >/dev/null; then break; fi
  sleep 1
done
curl --fail --silent "$API_URL/readyz" >"$ARTIFACT_DIR/readyz.json" || { compose logs --no-color >"$ARTIFACT_DIR/compose.log"; log "API did not become ready (see $ARTIFACT_DIR/compose.log)"; exit 1; }

# Insert deterministic smoke-owned records into this run's isolated Compose database.
compose exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO accounts (id,email,password_hash) VALUES ('d0000000-0000-4000-8000-000000000010','smoke@example.invalid','$2a$12$YFnANTVzpBZyF5Nv7KGX9.yAHCBvay6/hL8uKEF336X0dmSMHrfwS');
INSERT INTO accounts (id,email,password_hash) VALUES ('d0000000-0000-4000-8000-000000000013','invitee@example.invalid','$2a$12$YFnANTVzpBZyF5Nv7KGX9.yAHCBvay6/hL8uKEF336X0dmSMHrfwS');
INSERT INTO organizations (id,name) VALUES ('d0000000-0000-4000-8000-000000000011','Smoke tenant');
INSERT INTO organization_memberships (id,organization_id,account_id,role) VALUES ('d0000000-0000-4000-8000-000000000012','d0000000-0000-4000-8000-000000000011','d0000000-0000-4000-8000-000000000010','owner');
INSERT INTO repositories (id,provider,full_name,webhook_secret_env_ref,organization_id) VALUES ('d0000000-0000-4000-8000-000000000001','github','smoke/agent-ops','SMOKE_WEBHOOK_SECRET','d0000000-0000-4000-8000-000000000011');
INSERT INTO issues (id,repository_id,number,title,state,status,labels) VALUES ('d0000000-0000-4000-8000-000000000002','d0000000-0000-4000-8000-000000000001',99001,'Smoke fixture: board serves real API data','open','review','["status:review"]');
INSERT INTO pull_requests (id,repository_id,number,title,state,head_sha,branch,closes) VALUES ('d0000000-0000-4000-8000-000000000003','d0000000-0000-4000-8000-000000000001',99002,'Smoke fixture: linked pull request','open','d000000000000000000000000000000000000000','smoke/real-data','[99001]');
INSERT INTO pipeline_runs (id,repository_id,external_id,pr_number,sha,status,conclusion) VALUES ('d0000000-0000-4000-8000-000000000004','d0000000-0000-4000-8000-000000000001','smoke-ci-99002',99002,'d000000000000000000000000000000000000000','completed','success');
SQL

# Exercise real account authentication and the supported organization-selection endpoint.
login=$(curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"email":"smoke@example.invalid","password":"smoke-password-not-a-secret"}' "$API_URL/api/v1/auth/login")
account_token=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' <<<"$login")
selected=$(curl --fail --silent --show-error -H "Authorization: Bearer $account_token" -H 'Content-Type: application/json' -d '{"organization_id":"d0000000-0000-4000-8000-000000000011"}' "$API_URL/api/v1/auth/select-organization")
SESSION_TOKEN=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' <<<"$selected")
session=$(curl --fail --silent --show-error -H "Authorization: Bearer $SESSION_TOKEN" "$API_URL/api/v1/auth/session")
python3 -c 'import json,sys; x=json.load(sys.stdin); assert x["account_id"] == "d0000000-0000-4000-8000-000000000010" and x["organization_id"] == "d0000000-0000-4000-8000-000000000011"' <<<"$session"
export SESSION_TOKEN
export SMOKE_API_URL="$API_URL"
export SMOKE_ORGANIZATION_ID=d0000000-0000-4000-8000-000000000011
export SMOKE_WEBHOOK_SECRET=fixture-webhook-secret
export SCM_REPOSITORIES_JSON='[{"provider":"gitlab","full_name":"smoke/group/project","token_env":"SMOKE_BACKFILL_TOKEN","webhook_secret_env":"SMOKE_WEBHOOK_SECRET","base_url_env":"SMOKE_GITLAB_FIXTURE_URL"}]'
export SMOKE_BACKFILL_TOKEN=fixture-backfill-token

# Exercise the invitation contract against the same real API/PostgreSQL data.
invite=$(curl --fail --silent --show-error -H "Authorization: Bearer $SESSION_TOKEN" -H 'Content-Type: application/json' \
  -d '{"role":"viewer","expires_in_seconds":3600}' "$API_URL/api/v1/organizations/d0000000-0000-4000-8000-000000000011/invitations")
invite_token=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])' <<<"$invite")
invitee_login=$(curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"email":"invitee@example.invalid","password":"smoke-password-not-a-secret"}' "$API_URL/api/v1/auth/login")
invitee_token=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' <<<"$invitee_login")
accepted=$(curl --fail --silent --show-error -H "Authorization: Bearer $invitee_token" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$invite_token\"}" "$API_URL/api/v1/organization-invitations/accept")
python3 -c 'import json,sys; x=json.load(sys.stdin); assert x["data"]["account_id"] == "d0000000-0000-4000-8000-000000000013" and x["data"]["role"] == "viewer"' <<<"$accepted"
if curl --silent --show-error -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $invitee_token" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$invite_token\"}" "$API_URL/api/v1/organization-invitations/accept" | grep -q '^409$'; then
  log "invitation replay rejected with 409"
else
  log "invitation replay was not rejected"; exit 1
fi
membership_count=$(compose exec -T postgres psql -U agent_ops -d agent_ops -At -c \
  "SELECT count(*) FROM organization_memberships WHERE organization_id='d0000000-0000-4000-8000-000000000011' AND account_id='d0000000-0000-4000-8000-000000000013' AND role='viewer'")
[[ "$(tr -d '[:space:]' <<<"$membership_count")" == "1" ]] || { log "invitation membership persistence check failed"; exit 1; }
log "invitation HTTP creation/acceptance/replay and persisted membership verified"

FIXTURE_PORT=${SMOKE_FIXTURE_PORT:-18081}
if lsof -nP -iTCP:"$FIXTURE_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then log "fixture port already in use: $FIXTURE_PORT"; exit 1; fi
FIXTURE_LOG="$ARTIFACT_DIR/gitlab-fixture.jsonl"
python3 "$ROOT/scripts/smoke.gitlab-fixture.py" "$FIXTURE_PORT" "$FIXTURE_LOG" >"$ARTIFACT_DIR/gitlab-fixture.log" 2>&1 &
FIXTURE_PID=$!
for _ in $(seq 1 30); do curl --silent "http://127.0.0.1:$FIXTURE_PORT/health" >/dev/null && break; sleep 0.1; done
FIXTURE_URL="$SMOKE_GITLAB_FIXTURE_URL"
# Legacy registration seeds a disposable GitLab fixture. Settings browser/API lifecycle separately
# verifies encrypted credential writes using the organization-owned settings endpoint.
FIXTURE_REPO_ID=d0000000-0000-4000-8000-000000000020
compose exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "INSERT INTO repositories (id,provider,full_name,webhook_secret_env_ref,organization_id,git_lab_base_url,status_mappings,backfill_token_ciphertext,webhook_secret_ciphertext) VALUES ('$FIXTURE_REPO_ID','gitlab','smoke/group/project','SMOKE_WEBHOOK_SECRET','d0000000-0000-4000-8000-000000000011','$FIXTURE_URL','{}','','')"

# Exercise the webhook's decrypted credential consumer and verify the fixture receives the exact token.
webhook_status=$(curl --silent --output /dev/null --write-out '%{http_code}' -X POST -H 'X-Gitlab-Token: fixture-webhook-secret' \
  -H 'X-Gitlab-Event: Issue Hook' -H 'Content-Type: application/json' -H 'X-Gitlab-Event-UUID: smoke-runtime-webhook' \
  -d '{"object_kind":"issue","project":{"path_with_namespace":"smoke/group/project"},"object_attributes":{"iid":1,"title":"fixture","state":"opened","updated_at":"2026-01-01T00:00:00Z"}}' \
  "$API_URL/api/v1/webhooks/gitlab")
printf '%s\n' "$webhook_status" >"$ARTIFACT_DIR/webhook-status.txt"
[[ "$webhook_status" == 200 ]] || { log "webhook credential consumer returned HTTP $webhook_status"; exit 1; }

log "fixture repository created and webhook credential consumer invoked"
python3 - "$FIXTURE_LOG" <<'PY'
import json,sys
events=[json.loads(line) for line in open(sys.argv[1], encoding='utf-8')]
assert events and all(e['token']=='fixture-backfill-token' for e in events), events
assert {e['path'].split('?')[0].rsplit('/',1)[-1] for e in events} >= {'issues','merge_requests','pipelines'}, events
print('fixture observed encrypted setting backfill token and all GitLab API consumers')
PY

run_browser() {
  local control=${1:-0}
  local filter=()
  if [[ $control -ne 0 ]]; then filter=(--grep-invert 'creates lists edits and deletes both settings'); fi
  if ! SMOKE_ARTIFACT_DIR="$ARTIFACT_DIR" SMOKE_CLIENT_URL="$CLIENT_URL" APP_API_URL="${SMOKE_BROWSER_API_URL:-$API_URL}" SMOKE_CONTROL_NO_SESSION="$control" SESSION_TOKEN="$SESSION_TOKEN" SMOKE_ORGANIZATION_ID="$SMOKE_ORGANIZATION_ID" SMOKE_API_URL="$API_URL" npx playwright test --config="$ROOT/scripts/smoke.playwright.config.ts" --reporter=line "${filter[@]}" 2>&1 | tee -a "$ARTIFACT_DIR/browser${control}.log"; then
    return 1
  fi
}

pnpm --dir "$CLIENT" install --frozen-lockfile
(cd "$CLIENT" && APP_API_URL="${SMOKE_BROWSER_API_URL:-$API_URL}" pnpm build)
(cd "$CLIENT" && APP_API_URL="$API_URL" exec pnpm preview --host 127.0.0.1 --port "$CLIENT_PORT") >"$ARTIFACT_DIR/preview.log" 2>&1 &
PREVIEW_PID=$!
log "preview pid $PREVIEW_PID on $CLIENT_PORT"
for _ in $(seq 1 30); do
  if ! kill -0 "$PREVIEW_PID" 2>/dev/null; then log "preview process exited during startup (see $ARTIFACT_DIR/preview.log)"; exit 1; fi
  if curl --fail --silent "$CLIENT_URL/" >/dev/null; then break; fi
  sleep 1
done
for _ in $(seq 1 60); do curl --fail --silent "$CLIENT_URL/board" >/dev/null && break; sleep 1; done
curl --fail --silent "$CLIENT_URL/board" >/dev/null || { log "client preview did not become ready"; exit 1; }

cd "$CLIENT"
: >"$ARTIFACT_DIR/browser0.log"
: >"$ARTIFACT_DIR/browser1.log"
if ! run_browser 0; then
  exit 1
fi
if run_browser 1; then
  log "session-removal control unexpectedly passed unchanged baseline assertions"
  exit 1
fi
if ! grep -E 'Expected: visible|Timeout .*exceeded|Issue board|Repository settings' "$ARTIFACT_DIR/browser1.log" >/dev/null; then
  log "session-removal control failed without the expected baseline auth/data diagnostic (see $ARTIFACT_DIR/browser1.log)"
  exit 1
fi
log "EXPECTED SESSION-REMOVAL CONTROL: unchanged browser assertions failed as unauthenticated (see $ARTIFACT_DIR/browser1.log)"
if ! run_browser 0; then
  log "restored auth smoke failed (see $ARTIFACT_DIR/browser0.log)"
  exit 1
fi
compose logs --no-color >"$ARTIFACT_DIR/compose.log"
log "browser evidence: $ARTIFACT_DIR/browser0.log (baseline and restored); session-removal control: $ARTIFACT_DIR/browser1.log; screenshot: $ARTIFACT_DIR/board.png"
