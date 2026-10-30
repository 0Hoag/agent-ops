#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TMP_DIR=$(mktemp -d)
RUN_ID="api-container-${USER:-user}-$(date +%s)-$$"
NETWORK="${RUN_ID}-net"; DB="${RUN_ID}-db"; API="${RUN_ID}-api"; IMAGE="${RUN_ID}:test"
API_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
cleanup() { docker rm -f "$API" "$DB" >/dev/null 2>&1 || true; docker network rm "$NETWORK" >/dev/null 2>&1 || true; docker image rm "$IMAGE" "${IMAGE}-root-control" >/dev/null 2>&1 || true; rm -rf "$TMP_DIR"; }
trap cleanup EXIT

docker network create "$NETWORK" >/dev/null
docker build -t "$IMAGE" "$ROOT/api" >/dev/null
sed 's/^USER 10001:10001$/USER root/' "$ROOT/api/Dockerfile" >"$TMP_DIR/Dockerfile.root"
docker build -f "$TMP_DIR/Dockerfile.root" -t "${IMAGE}-root-control" "$ROOT/api" >/dev/null
control_uid=$(docker inspect --format '{{.Config.User}}' "${IMAGE}-root-control")
if [ "$control_uid" = 10001 ]; then
  echo "FAIL: USER root control retained the production UID (got ${control_uid})" >&2
  exit 1
fi
printf 'PASS: USER root control rejected by UID assertion (diagnostic UID=%s)\n' "$control_uid"
docker run -d --name "$DB" --network "$NETWORK" -e POSTGRES_USER=agent_ops -e POSTGRES_PASSWORD=fixture_password -e POSTGRES_DB=agent_ops postgres:17-alpine
db_ready=0
for _ in $(seq 1 60); do docker exec "$DB" pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && { db_ready=1; break; }; sleep 1; done
if [ "$db_ready" != 1 ]; then docker logs "$DB" >&2; exit 1; fi
docker run -d --name "$API" --network "$NETWORK" -p "127.0.0.1:${API_PORT}:8080" -e DATABASE_URL="postgres://agent_ops:fixture_password@${DB}:5432/agent_ops?sslmode=disable" -e JWT_SECRET=fixture-jwt -e REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 -e HTTP_ADDR=:8080 -e SCM_REPOSITORIES_JSON='[]' "$IMAGE"
api_ready=0
for _ in $(seq 1 90); do curl -fsS "http://127.0.0.1:${API_PORT}/readyz" >/dev/null 2>&1 && { api_ready=1; break; }; sleep 1; done
if [ "$api_ready" != 1 ]; then docker logs "$API" >&2; exit 1; fi
curl -fsS "http://127.0.0.1:${API_PORT}/healthz" >/dev/null
curl -fsS "http://127.0.0.1:${API_PORT}/readyz" >/dev/null
uid=$(docker exec "$API" id -u); test "$uid" != 0; test "$uid" = 10001
health=starting
for _ in $(seq 1 60); do health=$(docker inspect --format '{{.State.Health.Status}}' "$API"); [ "$health" = healthy ] && break; sleep 1; done
test "$health" = healthy
docker exec "$API" test -f /etc/ssl/certs/ca-certificates.crt
! docker exec "$API" sh -c 'command -v go'
printf 'PASS: image build, UID=%s, /healthz=200, /readyz=200, health=healthy, CA certs present, Go compiler absent\n' "$uid"
cleanup; trap - EXIT
if docker container inspect "$API" >/dev/null 2>&1 || docker container inspect "$DB" >/dev/null 2>&1 || docker network inspect "$NETWORK" >/dev/null 2>&1; then echo 'FAIL: fixture cleanup left an owned resource' >&2; exit 1; fi
printf 'PASS: cleanup verified (containers/network removed)\n'
