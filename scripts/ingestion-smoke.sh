#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
RUN_ID="w1e-${USER:-user}-$$-${RANDOM}"
CONTAINER="${RUN_ID}-postgres"
WORK=$(mktemp -d "${TMPDIR:-/tmp}/${RUN_ID}.XXXXXX")
API_PID=
VOLUME_ID=
CONTAINER_ID=
BASE_URL=

fail() { printf 'ASSERTION FAILED: %s\n' "$*" >&2; exit 1; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  local api_was_owned=$API_PID container_was_owned=$CONTAINER_ID volume_was_owned=$VOLUME_ID
  local cleanup_status=0
  local container_was_running=0
  if [[ -n "$API_PID" ]]; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
    API_PID=
  fi
  if [[ -n "$CONTAINER_ID" ]]; then
    docker inspect -f '{{.State.Running}}' "$CONTAINER_ID" 2>/dev/null | grep -q true && container_was_running=1 || true
    docker rm -f -v "$CONTAINER_ID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$VOLUME_ID" ]]; then
    docker volume rm "$VOLUME_ID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$container_was_owned" ]] && docker inspect "$container_was_owned" >/dev/null 2>&1; then
    printf 'CLEANUP FAILED: container remains: %s\n' "$container_was_owned" >&2; status=1; cleanup_status=1
  fi
  if (( container_was_running )) && [[ -n "$container_was_owned" ]] && docker ps -q --no-trunc --filter "id=$container_was_owned" | grep -q .; then
    printf 'CLEANUP FAILED: container still running: %s\n' "$container_was_owned" >&2; status=1; cleanup_status=1
  fi
  if [[ -n "$volume_was_owned" ]] && docker volume inspect "$volume_was_owned" >/dev/null 2>&1; then
    printf 'CLEANUP FAILED: volume remains: %s\n' "$volume_was_owned" >&2; status=1; cleanup_status=1
  fi
  if [[ -n "$api_was_owned" ]] && kill -0 "$api_was_owned" 2>/dev/null; then
    printf 'CLEANUP FAILED: API PID remains: %s\n' "$api_was_owned" >&2; status=1
    cleanup_status=1
  fi
  if [[ -f "$WORK/containers.before" ]]; then
    docker ps -aq --no-trunc | sort >"$WORK/containers.after"
    comm -13 "$WORK/containers.before" "$WORK/containers.after" >"$WORK/new-containers"
    if [[ -s "$WORK/new-containers" ]]; then printf 'CLEANUP FAILED: unexpected container(s) remain: %s\n' "$(tr '\n' ' ' <"$WORK/new-containers")" >&2; status=1; cleanup_status=1; fi
    docker volume ls -q | sort >"$WORK/volumes.after"
    comm -13 "$WORK/volumes.before" "$WORK/volumes.after" >"$WORK/new-volumes"
    if [[ -s "$WORK/new-volumes" ]]; then printf 'CLEANUP FAILED: unexpected volume(s) remain: %s\n' "$(tr '\n' ' ' <"$WORK/new-volumes")" >&2; status=1; cleanup_status=1; fi
  fi
  rm -rf "$WORK"
  if [[ -n "$container_was_owned" || -n "$volume_was_owned" || -n "$api_was_owned" ]]; then
    if (( cleanup_status == 0 )); then printf 'CLEANUP VERIFIED: API PID %s absent; container %s and volume %s removed\n' "${api_was_owned:-not-started}" "${container_was_owned:-not-created}" "${volume_was_owned:-not-created}"; fi
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

for tool in docker go curl python3 openssl; do command -v "$tool" >/dev/null || fail "required command unavailable: $tool"; done
docker info >/dev/null || fail 'Docker daemon unavailable'

# Capture service identities before creating anything; this script never prunes/stops by pattern.
docker ps -aq --no-trunc | sort >"$WORK/containers.before"
docker volume ls -q | sort >"$WORK/volumes.before"

# Establish cleanup ownership before creating the fixture resource.
CONTAINER_ID=$CONTAINER
docker run -d --name "$CONTAINER" -p "127.0.0.1::5432" -e POSTGRES_USER=w1e -e POSTGRES_PASSWORD=w1e-local-only \
  -e POSTGRES_DB=w1e postgres:17-alpine >"$WORK/container.id"
CONTAINER_ID=$(<"$WORK/container.id")
VOLUME_ID=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Name}}{{end}}{{end}}' "$CONTAINER_ID")
[[ -n "$VOLUME_ID" ]] || fail 'could not identify owned anonymous PostgreSQL volume'
DB_PORT=$(docker port "$CONTAINER_ID" 5432/tcp | python3 -c 'import sys; print(sys.stdin.read().strip().rsplit(":", 1)[1])')
for _ in {1..60}; do
  if docker exec "$CONTAINER_ID" pg_isready -U w1e -d w1e >/dev/null 2>&1; then break; fi
  sleep 1
done
docker exec "$CONTAINER_ID" pg_isready -U w1e -d w1e >/dev/null || fail 'PostgreSQL readiness timed out'

API_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
BASE_URL="http://127.0.0.1:$API_PORT"
SECRET_A="w1e-local-A-${RUN_ID}"
SECRET_B="w1e-local-B-${RUN_ID}"
CONFIG='[{"provider":"github","full_name":"w1e/a","webhook_secret_env":"W1E_A_SECRET"},{"provider":"github","full_name":"w1e/b","webhook_secret_env":"W1E_B_SECRET"}]'
(cd "$ROOT/api" && go build -o "$WORK/api" .)
HTTP_ADDR="127.0.0.1:$API_PORT" DATABASE_URL="postgres://w1e:w1e-local-only@127.0.0.1:$DB_PORT/w1e?sslmode=disable" \
SCM_REPOSITORIES_JSON="$CONFIG" W1E_A_SECRET="$SECRET_A" W1E_B_SECRET="$SECRET_B" \
  "$WORK/api" >"$WORK/api.log" 2>&1 &
API_PID=$!
for _ in {1..60}; do
  if curl --silent --fail "$BASE_URL/readyz" >/dev/null; then break; fi
  kill -0 "$API_PID" 2>/dev/null || { printf 'API exited during startup\n' >&2; exit 1; }
  sleep 1
done
curl --silent --fail "$BASE_URL/readyz" >/dev/null || fail 'API readiness timed out'

payload() { python3 -c 'import json,sys; print(json.dumps({"repository":{"full_name":sys.argv[1]},"issue":{"id":910,"number":910,"title":"w1e smoke","state":"closed","labels":[{"name":"status:review"}]}} ,separators=(",",":")))' "$1"; }
send() {
  local repo=$1 delivery=$2 secret=$3 body sig
  body=$(payload "$repo")
  sig=$(printf %s "$body" | openssl dgst -sha256 -hmac "$secret" -hex | python3 -c 'import sys; print(sys.stdin.read().strip().split()[-1])')
  curl --silent --show-error --output "$WORK/response" --write-out '%{http_code}' \
    -H 'Content-Type: application/json' -H 'X-GitHub-Event: issues' -H "X-GitHub-Delivery: $delivery" \
    -H "X-Hub-Signature-256: sha256=$sig" --data-binary "$body" "$BASE_URL/api/v1/webhooks/github"
}
assert_response() {
  local status=$1 accepted=$2 got
  got=$(python3 - "$WORK/response" "$accepted" <<'PY'
import json,sys
body=json.load(open(sys.argv[1])); expected=sys.argv[2]
if expected != 'none' and body.get('accepted') is not (expected == 'true'):
    raise SystemExit(f"accepted={body.get('accepted')!r}, expected {expected}")
PY
  ) || fail "unexpected response body for HTTP $status"
  : "$got"
}
snapshot() {
  docker exec "$CONTAINER_ID" psql -U w1e -d w1e -At -F '|' -c \
    "SELECT 'raw|'||count(*) FROM webhook_events UNION ALL SELECT 'issue|'||count(*) FROM issues UNION ALL SELECT 'identity|'||r.full_name||':'||i.number||':'||i.state||':'||i.status FROM issues i JOIN repositories r ON r.id=i.repository_id ORDER BY 1"
}
initial=$(snapshot)
[[ "$initial" == $'issue|0\nraw|0' ]] || fail "fixture not empty after migration: $initial"

status=$(send w1e/a delivery-a-910 "$SECRET_A"); [[ "$status" == 200 ]] || fail "A/A HTTP $status"
assert_response "$status" true
if [[ ${W1E_EXTRA_ROW_CONTROL:-0} == 1 ]]; then
  docker exec "$CONTAINER_ID" psql -U w1e -d w1e -v ON_ERROR_STOP=1 -c \
    "INSERT INTO webhook_events (id,repository_id,provider,event_type,delivery_id,payload) SELECT gen_random_uuid(),id,'github','issues','control-extra-raw','{}'::jsonb FROM repositories WHERE full_name='w1e/a'; INSERT INTO issues (id,repository_id,number,title,state,status) SELECT gen_random_uuid(),id,911,'control extra row','closed','done' FROM repositories WHERE full_name='w1e/a';" \
    >/dev/null
fi
after_a=$(snapshot)
expected_after_a=$'identity|w1e/a:910:closed:done\nissue|1\nraw|1'
[[ "$after_a" == "$expected_after_a" ]] || fail "A/A persisted snapshot mismatch: expected [$expected_after_a], got [$after_a]"
printf 'A_A=200 accepted:true; SQL raw=1 issue=1 identity=w1e/a:910:closed:done\n'

status=$(send w1e/a delivery-b-a-910 "$SECRET_B"); [[ "$status" == 400 ]] || fail "B/A HTTP $status"
assert_response "$status" none
[[ "$(snapshot)" == "$after_a" ]] || fail 'B/A modified SQL rows'
printf 'B_A=400 rows unchanged\n'
status=$(send w1e/unknown delivery-unknown-910 "$SECRET_A"); [[ "$status" == 400 ]] || fail "signed unknown HTTP $status"
assert_response "$status" none
[[ "$(snapshot)" == "$after_a" ]] || fail 'signed unknown modified SQL rows'
printf 'SIGNED_UNKNOWN=400 rows unchanged\n'
status=$(send w1e/a delivery-a-910 "$SECRET_A"); [[ "$status" == 200 ]] || fail "duplicate HTTP $status"
assert_response "$status" false
[[ "$(snapshot)" == "$after_a" ]] || fail 'duplicate modified SQL rows'
printf 'DUPLICATE=200 accepted:false rows unchanged\n'

if [[ ${W1E_FORCE_ASSERTION_FAILURE:-0} == 1 ]]; then fail 'deliberate assertion failure control'; fi
printf 'SMOKE PASSED\n'
