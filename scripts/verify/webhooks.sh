#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${WEBHOOKS_VERIFY_ARTIFACT_DIR:-/Users/zeross/agent-ops/.agent-runs/W19d-webhooks-512}
RUN_DIR="$ROOT/.agent-runs/webhooks-verify-$$"
mkdir -p "$ARTIFACT_DIR" "$RUN_DIR"
mkdir -p "$RUN_DIR/buildx" "$RUN_DIR/cache" "$RUN_DIR/modcache"
export BUILDX_CONFIG="$RUN_DIR/buildx" GOCACHE="$RUN_DIR/cache" GOMODCACHE="$RUN_DIR/modcache"
PROJECT="agent-ops-webhooks-$(id -u)-$$"
API_PORT=${WEBHOOKS_VERIFY_API_PORT:-$((19800 + RANDOM % 700))}
PG_PORT=${WEBHOOKS_VERIFY_PG_PORT:-$((16800 + RANDOM % 700))}
API="http://127.0.0.1:$API_PORT"
KEY=01234567890123456789012345678901
GITHUB_REPO=verify/github-webhooks
GITLAB_REPO=verify/gitlab-webhooks
ORG=51200000-0000-4000-8000-000000000001
GITHUB_SECRET=fixture-webhook-secret
GITLAB_SECRET=fixture-backfill-token
COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=webhooks-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" SYNC_INTERVAL=1h \
  SCM_REPOSITORIES_JSON="[{\"provider\":\"github\",\"full_name\":\"$GITHUB_REPO\",\"organization_id\":\"$ORG\",\"webhook_secret_env\":\"SMOKE_WEBHOOK_SECRET\"},{\"provider\":\"gitlab\",\"full_name\":\"$GITLAB_REPO\",\"organization_id\":\"$ORG\",\"webhook_secret_env\":\"SMOKE_BACKFILL_TOKEN\"}]" \
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
BOOT_COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=webhooks-verify-jwt REPOSITORY_ENCRYPTION_KEY="$KEY" SCM_REPOSITORIES_JSON=[] \
  docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")

log() { printf 'webhooks: %s\n' "$*"; }
fail() { log "FAIL: $*"; exit 1; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || status=1
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then log "FAIL: owned containers remain"; status=1; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then log "FAIL: owned volumes remain"; status=1; fi
  if docker network ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then log "FAIL: owned networks remain"; status=1; fi
  rm -rf "$RUN_DIR"
  [[ $status -eq 0 ]] && log "cleanup verified: no owned containers, volumes, or networks ($PROJECT)"
  exit "$status"
}
trap cleanup EXIT INT TERM

request() { CODE=$(curl -sS -o "$RUN_DIR/body" -w '%{http_code}' "$@"); BODY=$(tr '\n' ' ' <"$RUN_DIR/body"); }
check() { [[ "$CODE" == "$1" ]] || fail "$2 expected HTTP $1, got $CODE body=$BODY"; log "PASS: $2 HTTP=$CODE body=$BODY"; }
sql() { "${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -v ON_ERROR_STOP=1 -c "$1" | tr -d '\r'; }
sign() { printf 'sha256=%s' "$(openssl dgst -sha256 -hmac "$2" "$1" | awk '{print $2}')"; }
post_github() { local file=$1 id=$2 sig; sig=$(sign "$file" "$GITHUB_SECRET"); request -X POST -H 'Content-Type: application/json' -H "X-GitHub-Delivery: $id" -H 'X-GitHub-Event: issues' -H "X-Hub-Signature-256: $sig" --data-binary "@$file" "$API/api/v1/webhooks/github"; }
post_gitlab() { local file=$1 id=$2 event=$3; request -X POST -H 'Content-Type: application/json' -H "X-Gitlab-Event-UUID: $id" -H "X-Gitlab-Event: $event" -H "X-Gitlab-Token: $GITLAB_SECRET" --data-binary "@$file" "$API/api/v1/webhooks/gitlab"; }

cat >"$RUN_DIR/github-new.json" <<EOF
{"repository":{"full_name":"$GITHUB_REPO"},"issue":{"id":1001,"number":7,"title":"newest valid issue","state":"open","updated_at":"2026-11-14T12:00:00Z","labels":[{"name":"status:review"}]}}
EOF
cat >"$RUN_DIR/github-old.json" <<EOF
{"repository":{"full_name":"$GITHUB_REPO"},"issue":{"id":1001,"number":7,"title":"older out of order issue","state":"open","updated_at":"2026-11-14T11:00:00Z","labels":[{"name":"status:blocked"}]}}
EOF
cat >"$RUN_DIR/github-unknown.json" <<EOF
{"repository":{"full_name":"$GITHUB_REPO"},"issue":{"id":1002,"number":8,"title":"must not persist","state":"open","updated_at":"2026-11-14T13:00:00Z"}}
EOF
cat >"$RUN_DIR/gitlab.json" <<EOF
{"object_attributes":{"id":2001,"iid":9,"title":"gitlab issue","state":"opened","updated_at":"2026-11-14T12:00:00Z","labels":["Status::Review"]},"project":{"id":2,"path_with_namespace":"$GITLAB_REPO"}}
EOF
printf '%2097153s' x >"$RUN_DIR/oversized.json"

"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d postgres
for _ in $(seq 1 60); do "${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
"${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null || fail 'postgres did not become ready'
"$ROOT/scripts/heavy.sh" "${BOOT_COMPOSE[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null || fail 'api did not become ready'
"${BOOT_COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "INSERT INTO organizations (id, name) VALUES ('$ORG', 'Webhook verification') ON CONFLICT (id) DO NOTHING;" >/dev/null
"${BOOT_COMPOSE[@]}" stop api >/dev/null
"${BOOT_COMPOSE[@]}" rm -f api >/dev/null
"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null || fail 'configured api did not become ready'
for _ in $(seq 1 30); do [[ "$(sql "SELECT count(*) FROM repositories WHERE full_name IN ('$GITHUB_REPO','$GITLAB_REPO');")" == 2 ]] && break; sleep 1; done
[[ "$(sql "SELECT count(*) FROM repositories WHERE full_name IN ('$GITHUB_REPO','$GITLAB_REPO');")" == 2 ]] || fail 'configured repositories were not persisted'
log "fixture refs=$(sql "SELECT provider || ':' || full_name || ':' || webhook_secret_env_ref FROM repositories WHERE full_name IN ('$GITHUB_REPO','$GITLAB_REPO') ORDER BY provider") api_secret_lengths=$("${COMPOSE[@]}" exec -T api sh -c 'printf %s "$SMOKE_WEBHOOK_SECRET:$SMOKE_BACKFILL_TOKEN" | wc -c' | tr -d '\r')"
log 'PASS: fresh isolated stack and configured repositories persisted'

post_github "$RUN_DIR/github-new.json" github-valid-1; check 200 'valid GitHub signature accepted'
[[ "$(sql "SELECT title || '|' || status || '|' || i.upstream_updated_at FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITHUB_REPO' AND i.number=7;")" == *'newest valid issue|review|2026-11-14 12:00:00'* ]] || fail "GitHub SQL state mismatch: $(sql "SELECT title || '|' || status || '|' || i.upstream_updated_at FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITHUB_REPO' AND i.number=7;")"
log "PASS: GitHub SQL state=$(sql "SELECT title || '|' || status || '|' || i.upstream_updated_at FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITHUB_REPO' AND i.number=7;")"
post_github "$RUN_DIR/github-new.json" github-valid-1; check 200 'replayed GitHub delivery accepted idempotently'
[[ "$(sql "SELECT count(*) FROM webhook_events e JOIN repositories r ON r.id=e.repository_id WHERE r.full_name='$GITHUB_REPO' AND e.delivery_id='github-valid-1';")" == 1 ]] || fail 'replayed delivery created more than one event'
[[ "$(sql "SELECT count(*) FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITHUB_REPO' AND i.number=7;")" == 1 ]] || fail 'replayed delivery changed issue cardinality'
log "PASS: replay SQL event_count=$(sql "SELECT count(*) FROM webhook_events e JOIN repositories r ON r.id=e.repository_id WHERE r.full_name='$GITHUB_REPO' AND e.delivery_id='github-valid-1';") issue_count=$(sql "SELECT count(*) FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITHUB_REPO' AND i.number=7;")"
post_github "$RUN_DIR/github-old.json" github-old-1; check 200 'older out-of-order GitHub delivery accepted without rollback'
[[ "$(sql "SELECT title || '|' || status || '|' || i.upstream_updated_at FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITHUB_REPO' AND i.number=7;")" == *'newest valid issue|review|2026-11-14 12:00:00'* ]] || fail 'out-of-order delivery rolled state back'
log "PASS: out-of-order SQL state preserved=$(sql "SELECT title || '|' || status || '|' || i.upstream_updated_at FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITHUB_REPO' AND i.number=7;")"

request -X POST -H 'Content-Type: application/json' -H 'X-GitHub-Delivery: github-invalid-1' -H 'X-GitHub-Event: issues' -H 'X-Hub-Signature-256: sha256=0000000000000000000000000000000000000000000000000000000000000000' --data-binary "@$RUN_DIR/github-new.json" "$API/api/v1/webhooks/github"; check 400 'invalid GitHub signature rejected'; [[ "$(sql "SELECT count(*) FROM webhook_events e WHERE e.delivery_id='github-invalid-1';")" == 0 ]] || fail 'invalid signature mutated webhook_events'; log 'PASS: invalid GitHub signature SQL event_count=0'
request -X POST -H 'Content-Type: application/json' -H 'X-GitHub-Delivery: github-unknown-1' -H 'X-GitHub-Event: push' -H "X-Hub-Signature-256: $(sign "$RUN_DIR/github-unknown.json" "$GITHUB_SECRET")" --data-binary "@$RUN_DIR/github-unknown.json" "$API/api/v1/webhooks/github"; check 400 'unknown GitHub event rejected'; [[ "$(sql "SELECT count(*) FROM webhook_events e WHERE e.delivery_id='github-unknown-1';")" == 0 ]] || fail 'unknown event mutated webhook_events'; log 'PASS: unknown GitHub event SQL event_count=0'
request -X POST -H 'Content-Type: application/json' -H 'X-GitHub-Delivery: github-large-1' -H 'X-GitHub-Event: issues' -H "X-Hub-Signature-256: sha256=invalid" --data-binary "@$RUN_DIR/oversized.json" "$API/api/v1/webhooks/github"; check 413 'oversized body rejected'; [[ "$(sql "SELECT count(*) FROM webhook_events e WHERE e.delivery_id='github-large-1';")" == 0 ]] || fail 'oversized body mutated webhook_events'; log 'PASS: oversized body SQL event_count=0'

post_gitlab "$RUN_DIR/gitlab.json" gitlab-valid-1 'Issue Hook'; check 200 'valid GitLab token accepted'
[[ "$(sql "SELECT title || '|' || status FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITLAB_REPO' AND i.number=9;")" == 'gitlab issue|review' ]] || fail 'GitLab SQL state mismatch'; log "PASS: GitLab SQL state=$(sql "SELECT title || '|' || status FROM issues i JOIN repositories r ON r.id=i.repository_id WHERE r.full_name='$GITLAB_REPO' AND i.number=9;")"
request -X POST -H 'Content-Type: application/json' -H 'X-Gitlab-Event-UUID: gitlab-invalid-1' -H 'X-Gitlab-Event: Issue Hook' -H 'X-Gitlab-Token: wrong-token' --data-binary "@$RUN_DIR/gitlab.json" "$API/api/v1/webhooks/gitlab"; check 400 'invalid GitLab token rejected'; [[ "$(sql "SELECT count(*) FROM webhook_events e WHERE e.delivery_id='gitlab-invalid-1';")" == 0 ]] || fail 'invalid GitLab token mutated webhook_events'; log 'PASS: invalid GitLab token SQL event_count=0'
log "PASS: all webhook acceptance checks completed at source=$(git -C "$ROOT" rev-parse HEAD)"
