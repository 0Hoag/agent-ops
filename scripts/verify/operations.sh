#!/usr/bin/env bash
set -Eeuo pipefail

# Real operational acceptance: production Compose, application backup/restore, and
# an upgrade from a caller-supplied released API tree. Run via scripts/heavy.sh.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
PROJECT="agent-ops-operations-${USER:-user}-$$"
CLIENT_DIR=${CLIENT_DIR:-$ROOT/../agent-ops-client}
PREVIOUS_RELEASE_REF=${PREVIOUS_RELEASE_REF:-}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/agent-ops-operations.XXXXXX")
ARCHIVE="$TMP/application.dump"
CORRUPT="$TMP/corrupt.dump"
API_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
CLIENT_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
DB_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
export COMPOSE_PROJECT_NAME=$PROJECT CLIENT_DIR POSTGRES_PASSWORD=operations-postgres-password
export JWT_SECRET=operations-jwt-secret REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901
export PRODUCTION_API_PORT=$API_PORT PRODUCTION_CLIENT_PORT=$CLIENT_PORT POSTGRES_HOST_PORT=$DB_PORT
CLEANUP_NEEDED=0

compose() { docker compose -p "$PROJECT" -f "$ROOT/compose.prod.yml" "$@"; }
psql_db() { compose exec -T postgres psql -U agent_ops -d "$1" -v ON_ERROR_STOP=1 "${@:2}"; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ $CLEANUP_NEEDED -eq 0 ]]; then
    rm -rf "$TMP"
    exit "$status"
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || status=$?
  local containers volumes
  containers=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")
  volumes=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT")
  if [[ -n $containers || -n $volumes ]]; then
    printf 'FAIL: owned resources remain (containers=%s volumes=%s)\n' "${containers:-none}" "${volumes:-none}" >&2
    status=1
  else
    printf 'PASS: cleanup verified zero owned containers/volumes (%s)\n' "$PROJECT"
  fi
  rm -rf "$TMP"
  exit "$status"
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null || { printf 'BLOCKED: docker is required\n' >&2; exit 2; }
if [[ -z $PREVIOUS_RELEASE_REF ]]; then
  printf 'BLOCKED: PREVIOUS_RELEASE_REF must identify an actual previous release; no usable release was discovered\n' >&2
  exit 2
fi
git -C "$ROOT" rev-parse --verify "$PREVIOUS_RELEASE_REF^{commit}" >/dev/null 2>&1 || {
  printf 'BLOCKED: PREVIOUS_RELEASE_REF does not resolve to a Git commit: %s\n' "$PREVIOUS_RELEASE_REF" >&2
  exit 2
}
[[ -d "$CLIENT_DIR" ]] || { printf 'BLOCKED: CLIENT_DIR is not a usable client checkout: %s\n' "$CLIENT_DIR" >&2; exit 2; }
git -C "$ROOT" archive --format=tar "$PREVIOUS_RELEASE_REF" api >"$TMP/previous-api.tar" || {
  printf 'BLOCKED: previous release has no usable api tree: %s\n' "$PREVIOUS_RELEASE_REF" >&2
  exit 2
}
mkdir "$TMP/previous-api"
tar -xf "$TMP/previous-api.tar" -C "$TMP/previous-api"
[[ -f "$TMP/previous-api/go.mod" && -f "$TMP/previous-api/main.go" ]] || {
  printf 'BLOCKED: previous release API tree cannot run migration machinery: %s\n' "$PREVIOUS_RELEASE_REF" >&2
  exit 2
}
CLEANUP_NEEDED=1

compose config --quiet
compose up --build --wait --wait-timeout 180
printf 'PASS: production Compose services reached healthy state\n'
curl --silent --show-error --fail --max-time 10 "http://127.0.0.1:${CLIENT_PORT}/" | grep -q '<html'
printf 'PASS: production client served through its real HTTP interface\n'

db="agent_ops"
psql_db "$db" -c "CREATE TABLE IF NOT EXISTS operations_fixture (id integer PRIMARY KEY, value text NOT NULL); TRUNCATE operations_fixture; INSERT INTO operations_fixture VALUES (1, 'persisted'), (2, 'checksum');"
before=$(compose exec -T postgres pg_dump -U agent_ops -d "$db" --data-only --column-inserts --no-owner --no-privileges | sha256sum | awk '{print $1}')
container=$(compose ps -q postgres)
docker cp "$ROOT/scripts/backup.sh" "$container:/tmp/backup.sh"
docker cp "$ROOT/scripts/restore.sh" "$container:/tmp/restore.sh"
docker exec "$container" chmod +x /tmp/backup.sh /tmp/restore.sh
source_url="postgresql://agent_ops:${POSTGRES_PASSWORD}@localhost/${db}"
target_db="${PROJECT//-/_}_restored"
psql_db postgres -c "CREATE DATABASE \"$target_db\""
target_url="postgresql://agent_ops:${POSTGRES_PASSWORD}@localhost/${target_db}"
docker exec "$container" /tmp/backup.sh "$source_url" /tmp/application.dump
docker exec "$container" cp /tmp/application.dump /tmp/corrupt.dump
docker exec "$container" sh -c 'truncate -s 17 /tmp/corrupt.dump'
if docker exec "$container" /tmp/restore.sh /tmp/corrupt.dump "$target_url"; then
  printf 'FAIL: changed-input corrupt archive unexpectedly restored\n' >&2; exit 1
fi
# The real archive is on the container; restore it into the empty target.
docker exec "$container" /tmp/restore.sh /tmp/application.dump "$target_url"
after=$(compose exec -T postgres pg_dump -U agent_ops -d "$target_db" --data-only --column-inserts --no-owner --no-privileges | sha256sum | awk '{print $1}')
[[ $before == "$after" ]] || { printf 'FAIL: populated database checksum mismatch before=%s after=%s\n' "$before" "$after" >&2; exit 1; }
printf 'PASS: populated application backup/restore checksum=%s\n' "$before"
if docker exec "$container" /tmp/restore.sh /tmp/application.dump "$target_url"; then
  printf 'FAIL: occupied target restore unexpectedly passed\n' >&2; exit 1
fi
printf 'PASS: occupied restore target refused\n'

# Upgrade a populated database: old release API applies its own migrations, then
# the current API applies the remaining migrations against the same database.
upgrade_db="${PROJECT//-/_}_upgrade"
psql_db postgres -c "CREATE DATABASE \"$upgrade_db\""
upgrade_url="postgresql://agent_ops:${POSTGRES_PASSWORD}@localhost/${upgrade_db}"
(cd "$TMP/previous-api" && go build -o "$TMP/previous-api-bin" .)
DATABASE_URL="$upgrade_url" JWT_SECRET=operations-jwt-secret REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 HTTP_ADDR=":$((API_PORT+1))" "$TMP/previous-api-bin" >"$TMP/previous-api.log" 2>&1 & old_pid=$!
for _ in $(seq 1 90); do curl --silent --fail "http://127.0.0.1:$((API_PORT+1))/readyz" >/dev/null 2>&1 && break; sleep 1; done
kill "$old_pid" 2>/dev/null || true; wait "$old_pid" 2>/dev/null || true
psql_db "$upgrade_db" -c "CREATE TABLE IF NOT EXISTS operations_upgrade_fixture (id integer PRIMARY KEY, value text NOT NULL); INSERT INTO operations_upgrade_fixture VALUES (1, 'from-previous-release');"
(cd "$ROOT/api" && go build -o "$TMP/current-api-bin" .)
DATABASE_URL="$upgrade_url" JWT_SECRET=operations-jwt-secret REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 HTTP_ADDR=":$((API_PORT+2))" "$TMP/current-api-bin" >"$TMP/current-api.log" 2>&1 & current_pid=$!
for _ in $(seq 1 90); do curl --silent --fail "http://127.0.0.1:$((API_PORT+2))/readyz" >/dev/null 2>&1 && break; sleep 1; done
kill "$current_pid" 2>/dev/null || true; wait "$current_pid" 2>/dev/null || true
upgrade_row=$(psql_db "$upgrade_db" -Atqc "SELECT value FROM operations_upgrade_fixture WHERE id=1")
max_migration=$(psql_db "$upgrade_db" -Atqc 'SELECT max(version) FROM schema_migrations')
[[ $upgrade_row == from-previous-release && $max_migration -ge 1 ]] || {
  printf 'FAIL: current migration did not preserve previous-release data (value=%s max_migration=%s)\n' "$upgrade_row" "$max_migration" >&2
  exit 1
}
printf 'PASS: previous release populated data and current release migrated it (ref=%s max_migration=%s)\n' "$PREVIOUS_RELEASE_REF" "$max_migration"
