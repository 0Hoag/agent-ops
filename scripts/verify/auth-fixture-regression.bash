#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
PROJECT="agent-ops-auth-fixture-regression-$(id -u)-$$"
PG_PORT=${AUTH_FIXTURE_REGRESSION_PG_PORT:-$((15432 + (${RANDOM:-1} % 1000)))}
COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" \
  JWT_SECRET=auth-fixture-regression-secret REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 \
  JWT_EXPIRATION=2s SCM_REPOSITORIES_JSON=[] docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || status=1
  [[ -z $(docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}') ]] || status=1
  [[ -z $(docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}') ]] || status=1
  exit "$status"
}
trap cleanup EXIT INT TERM

"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d postgres >/dev/null
for _ in $(seq 1 60); do
  "${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break
  sleep 1
done
"${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null
for migration in "$ROOT"/api/migrations/*.sql; do
  "${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 <"$migration" >/dev/null
done

set +e
original=$("${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 2>&1 <<'SQL'
INSERT INTO repositories (id,provider,full_name,organization_id)
VALUES ('d0000000-0000-4000-8000-000000000021','github','auth-fixture/original','d0000000-0000-4000-8000-000000000022');
SQL
)
original_rc=$?
set -e
[[ $original_rc -ne 0 ]] || { printf '%s\n' 'original fixture unexpectedly succeeded' >&2; exit 1; }
grep -Fq 'null value in column "webhook_secret_env_ref"' <<<"$original" || { printf '%s\n' "$original" >&2; exit 1; }
printf 'PASS original fixture fails NOT NULL control: %s\n' "$original" 

"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
INSERT INTO repositories (id,provider,full_name,organization_id,webhook_secret_env_ref)
VALUES ('d0000000-0000-4000-8000-000000000023','github','auth-fixture/fixed',NULL,'AUTH_VERIFY_SECRET_FIXED');
SQL
count=$("${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -c "SELECT count(*) FROM repositories WHERE full_name = 'auth-fixture/fixed' AND webhook_secret_env_ref = 'AUTH_VERIFY_SECRET_FIXED';")
[[ $count == 1 ]] || { printf 'fixed fixture row count=%s\n' "$count" >&2; exit 1; }
printf 'PASS fixed fixture succeeds with synthetic secret reference: row count=%s\n' "$count"
