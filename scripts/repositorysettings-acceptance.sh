#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-ops-w8d-repositorysettings-$(id -u)-$$"
PORT=${W8D_REPOSITORY_POSTGRES_PORT:-55435}
DSN="postgres://agent_ops:agent_ops_dev@127.0.0.1:${PORT}/agent_ops?sslmode=disable"
cleanup() {
  status=$?
  trap - EXIT INT TERM
  JWT_SECRET=w8d-disposable-secret POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" down --volumes --remove-orphans >/dev/null 2>&1 || true
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then status=1; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then status=1; fi
  exit "$status"
}
trap cleanup EXIT INT TERM
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then echo "port $PORT is occupied; choose W8D_REPOSITORY_POSTGRES_PORT" >&2; exit 1; fi
compose() { JWT_SECRET=w8d-disposable-secret POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" "$@"; }
compose up -d postgres >/dev/null
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null
run_test() { (cd "$ROOT/api" && W8D_REPOSITORY_ACCEPTANCE_DSN="$DSN" GOCACHE=/tmp/w8d-gocache go test -count=1 -run '^TestRepositorySettingsHTTPAcceptance$' ./modules/repositorysettings); }
run_control() { (cd "$1" && W8D_REPOSITORY_ACCEPTANCE_DSN="$DSN" W8D_REPOSITORY_ACCEPTANCE_CONTROL=role GOCACHE=/tmp/w8d-gocache go test -count=1 -run '^TestRepositorySettingsHTTPAcceptance$' ./modules/repositorysettings); }
echo 'BASELINE: repositorysettings HTTP CRUD, role, tenant, ciphertext and redaction acceptance'
run_test
CONTROL_ROOT=$(mktemp -d /tmp/w8d-repositorysettings-control.XXXXXX)
trap 'rm -rf "$CONTROL_ROOT"' RETURN
cp -a "$ROOT/api/." "$CONTROL_ROOT/"
perl -0pi -e 's/p, err := c\.orgs\.Membership/_, err := c.orgs.Membership/; s/\n\tif write && p\.Role != "owner" && p\.Role != "admin" \{\n\t\tresponse\.Forbidden\(ctx, "repository settings require owner or admin"\)\n\t\treturn "", false\n\t\}//' "$CONTROL_ROOT/modules/repositorysettings/controller.go"
echo 'CONTROL role: removed owner/admin enforcement must fail the forbidden assertion'
if run_control "$CONTROL_ROOT" >"$CONTROL_ROOT/control.log" 2>&1; then cat "$CONTROL_ROOT/control.log"; echo 'role control unexpectedly passed' >&2; exit 1; fi
cat "$CONTROL_ROOT/control.log"
grep -F 'expected: 403' "$CONTROL_ROOT/control.log" >/dev/null
rm -rf "$CONTROL_ROOT"
echo 'RESTORATION: production source restored; baseline rerun'
run_test
