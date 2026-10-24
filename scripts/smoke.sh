#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CLIENT=${CLIENT_DIR:-"$HOME/agent-ops-client"}
PROJECT="agent-ops-smoke-$(id -u)-$$"
API_PORT=${SMOKE_API_PORT:-18080}
CLIENT_PORT=${SMOKE_CLIENT_PORT:-4173}
API_URL="http://127.0.0.1:$API_PORT"
CLIENT_URL="http://127.0.0.1:$CLIENT_PORT"
PREVIEW_PID=
ARTIFACT_DIR=${SMOKE_ARTIFACT_DIR:-"$ROOT/.agent-runs/smoke"}

log() { printf 'smoke: %s\n' "$*"; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ -n "$PREVIEW_PID" ]]; then
    preview_tree=$(python3 -c 'import subprocess,sys; root=int(sys.argv[1]); seen=set(); stack=[root];
while stack:
 p=stack.pop();
 if p in seen: continue
 seen.add(p)
 out=subprocess.run(["pgrep","-P",str(p)],capture_output=True,text=True); stack.extend(int(x) for x in out.stdout.split())
print(" ".join(map(str, seen)))' "$PREVIEW_PID" 2>/dev/null || printf '%s' "$PREVIEW_PID")
    for child in $preview_tree; do kill "$child" 2>/dev/null || true; done
    wait "$PREVIEW_PID" 2>/dev/null || true
  fi
  if [[ -n "$PREVIEW_PID" ]] && lsof -nP -iTCP:"$CLIENT_PORT" -sTCP:LISTEN 2>/dev/null | grep -q .; then log "cleanup failed: preview listener remains on $CLIENT_PORT"; status=1; fi
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null 2>&1 || true
  if [[ $status -ne 0 ]]; then docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" logs --no-color >"$ARTIFACT_DIR/compose.log" 2>/dev/null || true; fi
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

POSTGRES_HOST_PORT= POSTGRES_PUBLISH= docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" up -d --build
for _ in $(seq 1 90); do
  if curl --fail --silent "$API_URL/readyz" >/dev/null; then break; fi
  sleep 1
done
curl --fail --silent "$API_URL/readyz" >"$ARTIFACT_DIR/readyz.json" || { docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" logs --no-color >"$ARTIFACT_DIR/compose.log"; log "API did not become ready (see $ARTIFACT_DIR/compose.log)"; exit 1; }

# Insert deterministic smoke-owned records into this run's isolated Compose database.
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO repositories (id,provider,full_name,webhook_secret_env_ref) VALUES ('d0000000-0000-4000-8000-000000000001','github','smoke/agent-ops','SMOKE_WEBHOOK_SECRET');
INSERT INTO issues (id,repository_id,number,title,state,status,labels) VALUES ('d0000000-0000-4000-8000-000000000002','d0000000-0000-4000-8000-000000000001',99001,'Smoke fixture: board serves real API data','open','review','["status:review"]');
INSERT INTO pull_requests (id,repository_id,number,title,state,head_sha,branch,closes) VALUES ('d0000000-0000-4000-8000-000000000003','d0000000-0000-4000-8000-000000000001',99002,'Smoke fixture: linked pull request','open','d000000000000000000000000000000000000000','smoke/real-data','[99001]');
INSERT INTO pipeline_runs (id,repository_id,external_id,pr_number,sha,status,conclusion) VALUES ('d0000000-0000-4000-8000-000000000004','d0000000-0000-4000-8000-000000000001','smoke-ci-99002',99002,'d000000000000000000000000000000000000000','completed','success');
SQL

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
if ! SMOKE_ARTIFACT_DIR="$ARTIFACT_DIR" SMOKE_CLIENT_URL="$CLIENT_URL" APP_API_URL="${SMOKE_BROWSER_API_URL:-$API_URL}" npx playwright test --config="$ROOT/scripts/smoke.playwright.config.ts" --reporter=line 2>&1 | tee "$ARTIFACT_DIR/browser.log"; then
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" logs --no-color >"$ARTIFACT_DIR/compose.log"
  exit 1
fi
docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" logs --no-color >"$ARTIFACT_DIR/compose.log"
log "browser evidence: $ARTIFACT_DIR/browser.log; screenshot: $ARTIFACT_DIR/board.png"
