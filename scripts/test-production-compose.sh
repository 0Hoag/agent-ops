#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-$ROOT/compose.prod.yml}
PROJECT="production-compose-${USER:-user}-$$"
export COMPOSE_PROJECT_NAME=$PROJECT

if [[ -z ${CLIENT_DIR:-} ]]; then printf 'FAIL: CLIENT_DIR is required\n' >&2; exit 1; fi
for command in docker curl jq; do command -v "$command" >/dev/null || { printf 'FAIL: missing %s\n' "$command" >&2; exit 1; }; done
port() { python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()'; }
export PRODUCTION_API_PORT=${PRODUCTION_API_PORT:-$(port)}
export PRODUCTION_CLIENT_PORT=${PRODUCTION_CLIENT_PORT:-$(port)}
export POSTGRES_HOST_PORT=${POSTGRES_HOST_PORT:-$(port)}
export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-fixture-postgres-password}
export JWT_SECRET=${JWT_SECRET:-fixture-jwt-secret}
export REPOSITORY_ENCRYPTION_KEY=${REPOSITORY_ENCRYPTION_KEY:-01234567890123456789012345678901}
export POSTGRES_USER=${POSTGRES_USER:-agent_ops} POSTGRES_DB=${POSTGRES_DB:-agent_ops}
compose() { "$ROOT/scripts/heavy.sh" docker compose -f "$COMPOSE_FILE" -p "$PROJECT" "$@"; }
cleanup() {
  local status=$?
  compose down --volumes --remove-orphans >/dev/null 2>&1 || status=$?
  local containers volumes
  containers=$("$ROOT/scripts/heavy.sh" docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")
  volumes=$("$ROOT/scripts/heavy.sh" docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT")
  if [[ -n $containers || -n $volumes ]]; then printf 'FAIL: cleanup left project resources (containers=%s volumes=%s)\n' "${containers:-none}" "${volumes:-none}" >&2; status=1; else printf 'PASS: cleanup removed only project %s containers, network, and volume\n' "$PROJECT"; fi
  return "$status"
}
trap cleanup EXIT
compose config --quiet
printf 'PASS: production Compose config is valid\n'
compose up --build --wait --wait-timeout 180
printf 'PASS: postgres, API, and client reached healthy state\n'
client_url="http://127.0.0.1:${PRODUCTION_CLIENT_PORT}"
api_body=$(mktemp)
trap 'rm -f "$api_body"; cleanup' EXIT
api_status=$(curl --silent --show-error --max-time 10 -o "$api_body" -w '%{http_code}' -X POST "$client_url/api/v1/auth/login" -H 'Content-Type: application/json' --data '{"email":"fixture@example.invalid","password":"wrong-password"}')
[[ $api_status == 401 || $api_status == 422 ]] || { printf 'FAIL: proxied API status=%s body=%s\n' "$api_status" "$(cat "$api_body")" >&2; exit 1; }
jq -e 'type == "object" and (.error != null or .message != null or .data != null)' "$api_body" >/dev/null || { printf 'FAIL: /api/v1/auth/login was not API JSON: %s\n' "$(cat "$api_body")" >&2; exit 1; }
printf 'PASS: client proxy reached real API /api/v1/auth/login (HTTP %s, JSON body)\n' "$api_status"
compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "CREATE TABLE IF NOT EXISTS production_compose_fixture (value text NOT NULL); TRUNCATE production_compose_fixture; INSERT INTO production_compose_fixture(value) VALUES ('persistent-fixture');" >/dev/null
compose restart postgres api >/dev/null
compose up --wait --wait-timeout 90 >/dev/null
persisted=$(compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc 'SELECT value FROM production_compose_fixture')
[[ $persisted == persistent-fixture ]] || { printf 'FAIL: fixture data did not persist across restart: %s\n' "$persisted" >&2; exit 1; }
printf 'PASS: disposable PostgreSQL fixture persisted across restart\n'
for service in api client; do user=$(compose exec -T "$service" id -u); [[ $user != 0 ]] || { printf 'FAIL: %s runs as root\n' "$service" >&2; exit 1; }; printf 'PASS: %s runtime user is non-root (uid %s)\n' "$service" "$user"; done
curl --silent --show-error --fail --max-time 10 "$client_url/" | grep -q '<html' || { printf 'FAIL: production SPA was not served\n' >&2; exit 1; }
printf 'PASS: production SPA served\n'
