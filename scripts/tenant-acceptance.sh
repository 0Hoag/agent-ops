#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-ops-w7m-tenant-$(id -u)-$$"
PORT=${W7M_TENANT_POSTGRES_PORT:-55432}
CACHE=${W7M_TENANT_GOCACHE:-/tmp/agent-ops-w7m-go-cache}
DSN="postgres://agent_ops:agent_ops_dev@127.0.0.1:${PORT}/agent_ops?sslmode=disable"
CONTROL_ROOT=
BOARD_SHA=
MACHINE_SHA=
compose() { JWT_SECRET="${JWT_SECRET:-w7m-disposable-secret}" POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" "$@"; }
cleanup() {
  status=$?
  trap - EXIT
  if [[ -n "$CONTROL_ROOT" ]]; then rm -rf "$CONTROL_ROOT"; fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then status=1; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then status=1; fi
  exit "$status"
}
trap cleanup EXIT INT TERM
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then echo "port $PORT is occupied; choose W7M_TENANT_POSTGRES_PORT" >&2; exit 1; fi
compose up -d postgres >/dev/null
ready=
for _ in $(seq 1 60); do
  if compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
[[ -n "$ready" ]] || { echo "PostgreSQL did not become ready" >&2; exit 1; }
run_acceptance() {
  local source=$1 control=${2:-}
  (cd "$source" && W7M_TENANT_ACCEPTANCE_DSN="$DSN" W7M_TENANT_ACCEPTANCE_CONTROL="$control" GOCACHE="$CACHE" go test -count=1 -run '^TestW7MTenantAcceptance$' ./...)
}

run_acceptance "$ROOT/api"

BOARD_SHA=$(sha256sum "$ROOT/api/modules/board/repository.go" | awk '{print $1}')
MACHINE_SHA=$(sha256sum "$ROOT/api/modules/agentrun/controller.go" | awk '{print $1}')

CONTROL_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/w7m-tenant-control.XXXXXX")
cp -a "$ROOT/api/." "$CONTROL_ROOT/"
cp "$ROOT/api/tenant_acceptance_test.go" "$CONTROL_ROOT/tenant_acceptance_test.go"

# Remove only the production board organization predicate in the disposable copy.
perl -0pi -e 's/(func \(r \*Repository\) RepositoryExists.*?q :=.*?)(\n\tif organization := organizationFromContext\(ctx\); organization != "" \{\n\t\tq = q\.Where\("organization_id = \?", organization\)\n\t\})/$1/s' "$CONTROL_ROOT/modules/board/repository.go"
perl -0pi -e 's/(func \(r \*Repository\) ListIssues.*?q :=.*?)(\n\tif organization := organizationFromContext\(ctx\); organization != "" \{\n\t\tq = q\.Joins\("JOIN repositories ON repositories\.id = issues\.repository_id"\)\.Where\("repositories\.organization_id = \?", organization\)\n\t\})/$1/s' "$CONTROL_ROOT/modules/board/repository.go"
echo "EXPECTED BOARD CONTROL: removing board organization predicate must fail with repository authorization/leak diagnostic"
if run_acceptance "$CONTROL_ROOT" board >"$CONTROL_ROOT/board-control.log" 2>&1; then
  cat "$CONTROL_ROOT/board-control.log"
  echo "board control unexpectedly passed" >&2
  exit 1
fi
cat "$CONTROL_ROOT/board-control.log"
grep -F 'expected: 404' "$CONTROL_ROOT/board-control.log" >/dev/null || { echo "board control lacked expected authorization diagnostic" >&2; exit 1; }
grep -F 'tenant-b issue' "$CONTROL_ROOT/board-control.log" >/dev/null || { echo "board control lacked expected data-leak diagnostic" >&2; exit 1; }

# Remove only the configured-repository guard in the disposable copy.
cp "$ROOT/api/modules/agentrun/controller.go" "$CONTROL_ROOT/modules/agentrun/controller.go"
perl -0pi -e 's/\n\tif c\.configured != nil \{.*?\n\t\}\n\tif in\.InputTokens/\n\tif in.InputTokens/s' "$CONTROL_ROOT/modules/agentrun/controller.go"
echo "EXPECTED MACHINE CONTROL: removing configured-repository guard must fail with authorization diagnostic"
if run_acceptance "$CONTROL_ROOT" machine >"$CONTROL_ROOT/machine-control.log" 2>&1; then
  cat "$CONTROL_ROOT/machine-control.log"
  echo "machine control unexpectedly passed" >&2
  exit 1
fi
cat "$CONTROL_ROOT/machine-control.log"
grep -F 'expected: 401' "$CONTROL_ROOT/machine-control.log" >/dev/null || { echo "machine control lacked expected authorization diagnostic" >&2; exit 1; }

test "$BOARD_SHA" = "$(sha256sum "$ROOT/api/modules/board/repository.go" | awk '{print $1}')"
test "$MACHINE_SHA" = "$(sha256sum "$ROOT/api/modules/agentrun/controller.go" | awk '{print $1}')"
echo "RESTORATION: production source hashes unchanged after disposable controls"
run_acceptance "$ROOT/api"
