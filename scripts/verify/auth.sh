#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
API_PORT=${AUTH_VERIFY_API_PORT:-$((18080 + (${RANDOM:-1} % 1000)))}
PG_PORT=${AUTH_VERIFY_PG_PORT:-$((15432 + (${RANDOM:-1} % 1000)))}
PROJECT="agent-ops-auth-$(id -u)-$$"
API="http://127.0.0.1:$API_PORT"
COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=auth-verify-disposable-secret REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 \
  JWT_EXPIRATION=2s SCM_REPOSITORIES_JSON=[] docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
ARTIFACT_DIR=${AUTH_VERIFY_ARTIFACT_DIR:-"$ROOT/.agent-runs/W19a-auth-509"}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/auth-verify.XXXXXX")
ACCOUNT_A=auth-a-$(date +%s)-$$@example.invalid
ACCOUNT_B=auth-b-$(date +%s)-$$@example.invalid
PASSWORD='auth-verify-password'
ORG_A=d0000000-0000-4000-8000-000000000011
ORG_B=d0000000-0000-4000-8000-000000000016

log() { printf 'auth: %s\n' "$*"; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || status=1
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then
    log "cleanup failed: owned containers remain"; status=1
  fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then
    log "cleanup failed: owned volumes remain"; status=1
  fi
  rm -rf "$TMP"
  [[ $status -eq 0 ]] && log "cleanup verified: no owned containers or volumes ($PROJECT)"
  exit "$status"
}
trap cleanup EXIT INT TERM

mkdir -p "$ARTIFACT_DIR"
log "source SHA $(git -C "$ROOT" rev-parse HEAD)"
"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d postgres
for _ in $(seq 1 60); do "${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
"${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null
"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null

printf '%s\n' "$PASSWORD" | "${COMPOSE[@]}" exec -T api /bootstrap-account --email "$ACCOUNT_A" >/dev/null
printf '%s\n' "$PASSWORD" | "${COMPOSE[@]}" exec -T api /bootstrap-account --email "$ACCOUNT_B" >/dev/null
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 >/dev/null <<SQL
INSERT INTO organizations (id,name) VALUES ('$ORG_A','Auth verification A'),('$ORG_B','Auth verification B');
INSERT INTO organization_memberships (id,organization_id,account_id,role)
SELECT 'd0000000-0000-4000-8000-000000000012','$ORG_A',id,'owner' FROM accounts WHERE email='$ACCOUNT_A';
INSERT INTO organization_memberships (id,organization_id,account_id,role)
SELECT 'd0000000-0000-4000-8000-000000000017','$ORG_B',id,'owner' FROM accounts WHERE email='$ACCOUNT_B';
INSERT INTO organization_memberships (id,organization_id,account_id,role)
SELECT 'd0000000-0000-4000-8000-000000000018','$ORG_B',id,'member' FROM accounts WHERE email='$ACCOUNT_A';
INSERT INTO repositories (id,provider,full_name,organization_id) VALUES
('d0000000-0000-4000-8000-000000000001','github','auth-verify/a','$ORG_A'),
('d0000000-0000-4000-8000-000000000002','github','auth-verify/b','$ORG_B');
SQL

status() { curl -sS -o "$TMP/body" -w '%{http_code}' "$@"; }
check() { local name=$1 expected=$2 actual=$3; [[ "$actual" == "$expected" ]] || { log "FAIL: $name expected HTTP $expected, got $actual ($(tr '\n' ' ' <"$TMP/body"))"; exit 1; }; log "PASS: $name HTTP $actual"; }
json() { python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1" <"$TMP/body"; }

code=$(status -H 'Content-Type: application/json' -d '{"email":"'$ACCOUNT_A'","password":"'$PASSWORD'"}' "$API/api/v1/auth/login"); check 'login with bootstrap account' 200 "$code"; TOKEN=$(json token)
code=$(status -H 'Content-Type: application/json' -d '{"email":"'$ACCOUNT_A'","password":"wrong-password"}' "$API/api/v1/auth/login"); check 'wrong password rejected' 401 "$code"
code=$(status -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"organization_id":"'$ORG_A'"}' "$API/api/v1/auth/select-organization"); check 'organization selection' 200 "$code"; TOKEN_A=$(json token)
code=$(status -H "Authorization: Bearer $TOKEN_A" "$API/api/v1/auth/session"); check 'selected session reports organization A' 200 "$code"; grep -q '"organization_id":"'$ORG_A'"' "$TMP/body"
code=$(status -H "Authorization: Bearer $TOKEN_A" "$API/api/v1/repositories"); check 'organization A can read its repositories' 200 "$code"; grep -q 'auth-verify/a' "$TMP/body"; ! grep -q 'auth-verify/b' "$TMP/body"
code=$(status -H "Authorization: Bearer $TOKEN_A" -H 'Content-Type: application/json' -d '{"organization_id":"'$ORG_B'"}' "$API/api/v1/auth/select-organization"); check 'organization switch' 200 "$code"; TOKEN_B=$(json token)
code=$(status -H "Authorization: Bearer $TOKEN_B" "$API/api/v1/auth/session"); check 'switched session reports organization B' 200 "$code"; grep -q '"organization_id":"'$ORG_B'"' "$TMP/body"
code=$(status -H "Authorization: Bearer $TOKEN_B" "$API/api/v1/repositories"); check 'organization B can read only its repositories' 200 "$code"; grep -q 'auth-verify/b' "$TMP/body"; ! grep -q 'auth-verify/a' "$TMP/body"
code=$(status -H "Authorization: Bearer $TOKEN_A" -H 'Content-Type: application/json' -d '{"organization_id":"'$ORG_B'"}' "$API/api/v1/auth/select-organization"); check 'repeated organization selection remains authorized' 200 "$code"
sleep 3
code=$(status -H "Authorization: Bearer $TOKEN_B" "$API/api/v1/auth/session"); check 'expired token rejected' 401 "$code"
code=$(status "$API/api/v1/repositories"); check 'signout token disposal rejects protected request' 401 "$code"
log 'PASS: organization-token isolation verified with identical repository request and isolated fixtures'
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -At -c 'SELECT count(*) FROM organizations' >"$ARTIFACT_DIR/organization-count.txt"
