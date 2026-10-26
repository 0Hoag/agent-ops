#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-ops-w7m-tenant-$(id -u)-$$"
PORT=${W7M_TENANT_POSTGRES_PORT:-55432}
CACHE=${W7M_TENANT_GOCACHE:-/tmp/agent-ops-w7m-go-cache}
DSN="postgres://agent_ops:agent_ops_dev@127.0.0.1:${PORT}/agent_ops?sslmode=disable"
compose() { JWT_SECRET="${JWT_SECRET:-w7m-disposable-secret}" POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PORT" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" "$@"; }
cleanup() {
  status=$?
  trap - EXIT
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then status=1; fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then status=1; fi
  exit "$status"
}
trap cleanup EXIT INT TERM
if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then echo "port $PORT is occupied; choose W7M_TENANT_POSTGRES_PORT" >&2; exit 1; fi
compose up -d postgres >/dev/null
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
(cd "$ROOT/api" && W7M_TENANT_ACCEPTANCE_DSN="$DSN" GOCACHE="$CACHE" go test -count=1 -run '^TestW7MTenantAcceptance$' ./...)
