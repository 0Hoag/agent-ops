#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
HEAVY=/Users/zeross/agent-ops/scripts/heavy.sh
ARTIFACT_DIR=${CONTROL_VERIFY_ARTIFACT_DIR:-$ROOT/.agent-runs/W19j-control-518/$(date +%Y%m%dT%H%M%S)-$$}
mkdir -p "$ARTIFACT_DIR"
RUN=$(mktemp -d "${TMPDIR:-/tmp}/control-518.XXXXXX")
mkdir -p "$RUN/buildx"
export BUILDX_CONFIG="$RUN/buildx"
PROJECT="control-518-$(id -u)-$$"
PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
cp -R "$ROOT/api" "$RUN/api"
cp "$ROOT/scripts/verify/control/stub.sh" "$RUN/stub.sh"
chmod 755 "$RUN" "$RUN/stub.sh"
cat > "$RUN/compose.yml" <<YAML
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: agent_ops
      POSTGRES_PASSWORD: agent_ops_dev
      POSTGRES_DB: agent_ops
    volumes: [data:/var/lib/postgresql/data]
  api:
    image: $PROJECT-api:baseline
    build: $RUN/api
    environment:
      DATABASE_URL: postgres://agent_ops:agent_ops_dev@postgres:5432/agent_ops?sslmode=disable
      JWT_SECRET: control-518-disposable-jwt
      WORKER_SCRIPT_ROOTS: /fixture
      WORKER_WORKTREE_ROOTS: /fixture
      WORKER_SCRIPTS_JSON: '{"stub":"/fixture/stub.sh"}'
      SCM_REPOSITORIES_JSON: '[]'
    volumes: ["$RUN/stub.sh:/fixture/stub.sh:ro"]
    ports: ["127.0.0.1:$PORT:8080"]
volumes:
  data:
YAML
compose() { "$HEAVY" docker compose -p "$PROJECT" -f "$RUN/compose.yml" "$@"; }
sql() { compose exec -T postgres psql -U agent_ops -d agent_ops -At -v ON_ERROR_STOP=1 -c "$1"; }
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  compose logs --no-color > "$ARTIFACT_DIR/stack.log" 2>&1 || true
  compose down --volumes --remove-orphans > "$ARTIFACT_DIR/cleanup.log" 2>&1 || status=1
  for kind in container volume network; do
    case $kind in
      container) left=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT");;
      volume) left=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT");;
      network) left=$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT");;
    esac
    if [[ -n $left ]]; then echo "FAIL: cleanup $kind=$left"; status=1; fi
  done
  echo "cleanup status=$status project=$PROJECT (stub processes confined to removed API container)" | tee -a "$ARTIFACT_DIR/cleanup.log"
  rm -rf "$RUN"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM
printf 'tested_sha=%s mutation=%s\n' "$(git -C "$ROOT" rev-parse HEAD)" "${CONTROL_VERIFY_MUTATION:-0}" | tee "$ARTIFACT_DIR/source.txt"
compose up -d postgres
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
compose up -d --build api > "$ARTIFACT_DIR/build.log" 2>&1
for _ in $(seq 1 90); do curl -fsS "http://127.0.0.1:$PORT/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "http://127.0.0.1:$PORT/readyz" >/dev/null
for who in owner member foreign; do
  compose exec -T api sh -c 'printf "%s\n" control-518-password | /bootstrap-account --email "$1"' sh "$who@control.invalid" >/dev/null
 done
sql "INSERT INTO organizations(id,name) VALUES ('51800000-0000-4000-8000-000000000001','Control A'),('51800000-0000-4000-8000-000000000002','Control B');
INSERT INTO organization_memberships(id,organization_id,account_id,role)
SELECT gen_random_uuid(), CASE WHEN email='foreign@control.invalid' THEN '51800000-0000-4000-8000-000000000002'::uuid ELSE '51800000-0000-4000-8000-000000000001'::uuid END,id,CASE WHEN email='member@control.invalid' THEN 'member' ELSE 'owner' END FROM accounts;" >/dev/null
python3 "$ROOT/scripts/verify/control/acceptance.py" "http://127.0.0.1:$PORT" "$ARTIFACT_DIR" | tee "$ARTIFACT_DIR/checks.log"
sql "SELECT action || '|' || outcome || '|' || job_id FROM worker_action_audits ORDER BY occurred_at,id" > "$ARTIFACT_DIR/audit.sql.txt"
[[ $(sql "SELECT count(*) FROM worker_action_audits WHERE outcome='accepted'") == 3 ]]
[[ $(sql "SELECT count(*) FROM worker_action_audits WHERE outcome='role_rejected'") == 2 ]]
echo 'PASS: Postgres stores three accepted actions and two role denials'
compose restart api >/dev/null
for _ in $(seq 1 90); do curl -fsS "http://127.0.0.1:$PORT/readyz" >/dev/null 2>&1 && break; sleep 1; done
python3 "$ROOT/scripts/verify/control/acceptance.py" "http://127.0.0.1:$PORT" "$ARTIFACT_DIR" persisted | tee -a "$ARTIFACT_DIR/checks.log"
source "$ROOT/scripts/verify/control/mutation.sh"
echo 'PASS: control acceptance, persisted audit, and identical-input mutation restored'
