#!/usr/bin/env bash
set -Eeuo pipefail

# Real HTTP acceptance for organization membership, invitations, and RBAC.
# The optional MEMBERS_CONTROL=1 mode is used by the acceptance harness to
# rebuild a disposable image with the member-role guard deliberately broken.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${MEMBERS_VERIFY_ARTIFACT_DIR:-$ROOT/.agent-runs/W19b-members-510}
RUN_DIR="$ROOT/.agent-runs/members-verify-$$"
mkdir -p "$ARTIFACT_DIR" "$RUN_DIR/gocache" "$RUN_DIR/gomodcache" "$RUN_DIR/buildx"
export GOCACHE="$RUN_DIR/gocache" GOMODCACHE="$RUN_DIR/gomodcache" BUILDX_CONFIG="$RUN_DIR/buildx"

port() { python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()'; }
API_PORT=${MEMBERS_VERIFY_API_PORT:-$(port)}
PG_PORT=${MEMBERS_VERIFY_PG_PORT:-$(port)}
PROJECT="agent-ops-members-$(id -u)-$$"
API="http://127.0.0.1:$API_PORT"
PASSWORD='members-verify-password'
COMPOSE=(env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" \
  JWT_SECRET=members-verify-disposable-secret REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 \
  SCM_REPOSITORIES_JSON=[] docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")
TMP=$(mktemp -d "${TMPDIR:-/tmp}/members-verify.XXXXXX")
status_file="$TMP/body"
LAST_BODY=''
CODE=''
log() { printf 'members: %s\n' "$*"; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || status=1
  local containers volumes networks
  containers=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")
  volumes=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT")
  networks=$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT")
  if [[ -n $containers || -n $volumes || -n $networks ]]; then
    log "FAIL: cleanup left owned resources (containers=${containers:-none} volumes=${volumes:-none} networks=${networks:-none})"
    status=1
  else
    log "PASS: cleanup verified zero owned containers, volumes, and networks ($PROJECT)"
  fi
  rm -rf "$TMP" "$RUN_DIR"
  exit "$status"
}
trap cleanup EXIT INT TERM

docker info >/dev/null
test -w "$GOCACHE" -a -w "$GOMODCACHE" -a -w "$BUILDX_CONFIG"
mkdir -p "$ARTIFACT_DIR"
log "source SHA $(git -C "$ROOT" rev-parse HEAD)"

"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d postgres
for _ in $(seq 1 90); do "${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
"${COMPOSE[@]}" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null
"$ROOT/scripts/heavy.sh" "${COMPOSE[@]}" up -d --build api
for _ in $(seq 1 90); do curl -fsS "$API/readyz" >/dev/null && break; sleep 1; done
curl -fsS "$API/readyz" >/dev/null

stamp=$(date +%s)-$$
OWNER="members-owner-$stamp@example.invalid"
ADMIN="members-admin-$stamp@example.invalid"
MEMBER="members-member-$stamp@example.invalid"
VIEWER="members-viewer-$stamp@example.invalid"
TARGET="members-target-$stamp@example.invalid"
INVITEE="members-invitee-$stamp@example.invalid"
EXPIRED="members-expired-$stamp@example.invalid"
for email in "$OWNER" "$ADMIN" "$MEMBER" "$VIEWER" "$TARGET" "$INVITEE" "$EXPIRED"; do
  printf '%s\n' "$PASSWORD" | "${COMPOSE[@]}" exec -T api /bootstrap-account --email "$email" >/dev/null
done
ids=()
for email in "$OWNER" "$ADMIN" "$MEMBER" "$VIEWER" "$TARGET" "$INVITEE" "$EXPIRED"; do
  ids+=("$("${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -Atc "SELECT id FROM accounts WHERE email='$email'" | tr -d '\r')")
done
OWNER_ID=${ids[0]}; ADMIN_ID=${ids[1]}; MEMBER_ID=${ids[2]}; VIEWER_ID=${ids[3]}; TARGET_ID=${ids[4]}; INVITEE_ID=${ids[5]}; EXPIRED_ID=${ids[6]}

request() {
  local method=$1 path=$2 body=${3:-} token=${4:-}
  if [[ ("$method" == GET || "$method" == DELETE) && -n $body && -z $token ]]; then token=$body; body=; fi
  local args=(-sS -o "$status_file" -w '%{http_code}' -X "$method" -H 'Content-Type: application/json')
  [[ -n $token ]] && args+=(-H "Authorization: Bearer $token")
  [[ -n $body ]] && args+=(--data "$body")
  CODE=$(curl "${args[@]}" "$API$path")
  LAST_BODY=$(cat "$status_file")
  cat "$status_file" >"$TMP/last-body"
}
check() {
  local name=$1 expected=$2 actual=$3
  printf '%s\t%s\t%s\t%s\n' "$name" "$expected" "$actual" "$(tr '\n' ' ' <"$TMP/last-body")" >>"$ARTIFACT_DIR/http-checks.tsv"
  [[ "$actual" == "$expected" ]] || { log "FAIL: $name expected HTTP $expected, got $actual ($(tr '\n' ' ' <"$TMP/last-body"))"; exit 1; }
  log "PASS: $name HTTP $actual"
}
json() { python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1" <"$status_file"; }
login() { request POST /api/v1/auth/login "{\"email\":\"$1\",\"password\":\"$PASSWORD\"}"; check "login $1" 200 "$CODE"; json token; }
select_org() { request POST /api/v1/auth/select-organization "{\"organization_id\":\"$ORG\"}" "$1"; check "select organization for $2" 200 "$CODE"; json token; }

: >"$ARTIFACT_DIR/http-checks.tsv"
OWNER_TOKEN=$(login "$OWNER")
request POST /api/v1/organizations '{"name":"Members verification organization"}' "$OWNER_TOKEN"; check 'owner creates organization' 201 "$CODE"; ORG=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])' <"$status_file")
OWNER_TOKEN=$(select_org "$OWNER_TOKEN" owner)
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 >/dev/null <<SQL
INSERT INTO organization_memberships (id,organization_id,account_id,role) VALUES
 (gen_random_uuid(),'$ORG','$ADMIN_ID','admin'),(gen_random_uuid(),'$ORG','$MEMBER_ID','member'),(gen_random_uuid(),'$ORG','$VIEWER_ID','viewer');
SQL
ADMIN_TOKEN=$(select_org "$(login "$ADMIN")" admin)
MEMBER_TOKEN=$(select_org "$(login "$MEMBER")" member)
VIEWER_TOKEN=$(select_org "$(login "$VIEWER")" viewer)
TARGET_TOKEN=$(select_org "$(login "$TARGET")" target)
INVITEE_TOKEN=$(login "$INVITEE")
EXPIRED_TOKEN=$(login "$EXPIRED")

request GET /api/v1/organizations "$OWNER_TOKEN"; check 'owner lists organizations' 200 "$CODE"
request GET "/api/v1/organizations/$ORG" "$MEMBER_TOKEN"; check 'member reads organization detail' 200 "$CODE"
request GET "/api/v1/organizations/$ORG/members" "$VIEWER_TOKEN"; check 'viewer reads members' 200 "$CODE"
request GET /api/v1/organizations "$TARGET_TOKEN"; check 'nonmember sees empty organization list' 200 "$CODE"
request GET "/api/v1/organizations/$ORG" "$TARGET_TOKEN"; check 'nonmember detail denied' 404 "$CODE"
request GET "/api/v1/organizations/$ORG/members" "$TARGET_TOKEN"; check 'nonmember member-list denied' 404 "$CODE"

request POST "/api/v1/organizations/$ORG/members" "{\"account_id\":\"$TARGET_ID\",\"role\":\"member\"}" "$ADMIN_TOKEN"; check 'admin adds member' 204 "$CODE"
request POST "/api/v1/organizations/$ORG/members" "{\"account_id\":\"$TARGET_ID\",\"role\":\"viewer\"}" "$MEMBER_TOKEN"; check 'member cannot add member' 403 "$CODE"
request POST "/api/v1/organizations/$ORG/members" "{\"account_id\":\"$TARGET_ID\",\"role\":\"admin\"}" "$ADMIN_TOKEN"; check 'admin cannot add admin' 403 "$CODE"
request POST "/api/v1/organizations/$ORG/members" "{\"account_id\":\"$TARGET_ID\",\"role\":\"owner\"}" "$OWNER_TOKEN"; check 'owner cannot assign owner' 400 "$CODE"
request PATCH "/api/v1/organizations/$ORG/members/$TARGET_ID" '{"role":"viewer"}' "$ADMIN_TOKEN"; check 'admin changes member to viewer' 204 "$CODE"
request PATCH "/api/v1/organizations/$ORG/members/$ADMIN_ID" '{"role":"member"}' "$ADMIN_TOKEN"; check 'admin cannot change admin' 403 "$CODE"
request PATCH "/api/v1/organizations/$ORG/members/$TARGET_ID" '{"role":"admin"}' "$ADMIN_TOKEN"; check 'admin cannot promote to admin' 403 "$CODE"
request PATCH "/api/v1/organizations/$ORG/members/$TARGET_ID" '{"role":"member"}' "$OWNER_TOKEN"; check 'owner changes viewer to member' 204 "$CODE"
request DELETE "/api/v1/organizations/$ORG/members/$TARGET_ID" "$MEMBER_TOKEN"; check 'member cannot remove member' 403 "$CODE"
request DELETE "/api/v1/organizations/$ORG/members/$TARGET_ID" "$OWNER_TOKEN"; check 'owner removes member' 204 "$CODE"
request DELETE "/api/v1/organizations/$ORG/members/$ADMIN_ID" "$ADMIN_TOKEN"; check 'admin cannot remove admin' 403 "$CODE"
request DELETE "/api/v1/organizations/$ORG/members/$ADMIN_ID" "$OWNER_TOKEN"; check 'owner removes admin' 204 "$CODE"

request POST "/api/v1/organizations/$ORG/invitations" '{"role":"member","expires_in_seconds":86400}' "$OWNER_TOKEN"; check 'owner creates member invitation' 201 "$CODE"; INVITE_TOKEN=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])' <"$status_file")
request POST /api/v1/organization-invitations/accept "{\"token\":\"$INVITE_TOKEN\"}" "$INVITEE_TOKEN"; check 'invitee accepts invitation' 201 "$CODE"
request POST /api/v1/organization-invitations/accept "{\"token\":\"$INVITE_TOKEN\"}" "$INVITEE_TOKEN"; check 'reused invitation rejected' 409 "$CODE"
request POST "/api/v1/organizations/$ORG/invitations" '{"role":"admin","expires_in_seconds":86400}' "$INVITEE_TOKEN"; check 'member cannot invite admin' 403 "$CODE"
request POST "/api/v1/organizations/$ORG/invitations" '{"role":"admin","expires_in_seconds":86400}' "$OWNER_TOKEN"; check 'owner creates admin invitation' 201 "$CODE"
request POST "/api/v1/organizations/$ORG/invitations" '{"role":"owner","expires_in_seconds":86400}' "$OWNER_TOKEN"; check 'owner invitation cannot grant owner' 400 "$CODE"
request POST "/api/v1/organizations/$ORG/invitations" '{"role":"member","expires_in_seconds":1}' "$OWNER_TOKEN"; check 'owner creates expiring invitation' 201 "$CODE"; EXPIRED_INVITE=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])' <"$status_file"); sleep 2
request POST /api/v1/organization-invitations/accept "{\"token\":\"$EXPIRED_INVITE\"}" "$EXPIRED_TOKEN"; check 'expired invitation rejected' 409 "$CODE"

# Last-owner protection is exercised after adding a second owner only through the
# database fixture, because the public API must never grant owner role.
"${COMPOSE[@]}" exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 >/dev/null <<SQL
INSERT INTO organization_memberships (id,organization_id,account_id,role) VALUES (gen_random_uuid(),'$ORG','$ADMIN_ID','owner');
SQL
request PATCH "/api/v1/organizations/$ORG/members/$OWNER_ID" '{"role":"member"}' "$OWNER_TOKEN"; check 'owner demotion preserves final owner' 409 "$CODE"
request DELETE "/api/v1/organizations/$ORG/members/$OWNER_ID" "$OWNER_TOKEN"; check 'owner removal preserves final owner' 409 "$CODE"
request PATCH "/api/v1/organizations/$ORG/members/$ADMIN_ID" '{"role":"member"}' "$OWNER_TOKEN"; check 'owner demotes second owner' 204 "$CODE"
request PATCH "/api/v1/organizations/$ORG/members/$OWNER_ID" '{"role":"member"}' "$OWNER_TOKEN"; check 'last owner demotion rejected' 409 "$CODE"

cp "$ARTIFACT_DIR/http-checks.tsv" "$ARTIFACT_DIR/http-checks-$(git -C "$ROOT" rev-parse --short HEAD).tsv"
log 'PASS: real HTTP membership, invitation, RBAC, and last-owner assertions completed'
