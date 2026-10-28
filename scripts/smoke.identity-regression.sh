#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CLIENT=${CLIENT_DIR:-"$HOME/agent-ops-client"}
API_PORT=${SMOKE_API_PORT:-55962}
CLIENT_PORT=${SMOKE_CLIENT_PORT:-55963}
POSTGRES_PORT=${POSTGRES_HOST_PORT:-55961}
FIXTURE_PORT=${SMOKE_FIXTURE_PORT:-55964}
JWT_SECRET=${JWT_SECRET:-fixture-jwt}
ENCRYPTION_KEY=${REPOSITORY_ENCRYPTION_KEY:-01234567890123456789012345678901}
ARTIFACT_DIR=${SMOKE_IDENTITY_ARTIFACT_DIR:-"$ROOT/.agent-runs/smoke-identity-regression"}
PROJECT="agent-ops-identity-$(id -u)-$$"
SPEC="$ROOT/scripts/smoke.browser.spec.ts"
CONFIG="$ROOT/scripts/smoke.playwright.config.ts"
TEMP=$(mktemp -d "${TMPDIR:-/tmp}/smoke-identity.XXXXXX")
PREVIEW_PID=
FIXTURE_PID=
ACTIVE_COMPOSE=()

compose() { "${ACTIVE_COMPOSE[@]}" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" "$@"; }
log() { printf 'identity: %s\n' "$*"; }

cleanup() {
  local status=$?
  if [[ $status -ne 0 ]]; then
    compose logs --no-color api >"$ARTIFACT_DIR/server.log" 2>&1 || true
    compose exec -T postgres psql -U agent_ops -d agent_ops -At -F '|' -c 'SELECT provider,full_name FROM repositories ORDER BY provider,full_name' >"$ARTIFACT_DIR/final-identities.txt" 2>/dev/null || true
  fi
  trap - EXIT INT TERM
  if [[ -n "$PREVIEW_PID" ]]; then
    pkill -TERM -P "$PREVIEW_PID" 2>/dev/null || true
    kill "$PREVIEW_PID" 2>/dev/null || true
    wait "$PREVIEW_PID" 2>/dev/null || true
  fi
  if [[ -n "$FIXTURE_PID" ]]; then
    kill "$FIXTURE_PID" 2>/dev/null || true
    wait "$FIXTURE_PID" 2>/dev/null || true
  fi
  if [[ -n "${ACTIVE_COMPOSE[*]}" ]]; then
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  if docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}' | grep -q .; then
    log "cleanup failed: containers remain for $PROJECT"; status=1
  fi
  if docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}' | grep -q .; then
    log "cleanup failed: volumes remain for $PROJECT"; status=1
  fi
  for port in "$API_PORT" "$CLIENT_PORT" "$POSTGRES_PORT" "$FIXTURE_PORT"; do
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | grep -q .; then
      log "cleanup failed: listener remains on port $port"; status=1
    fi
  done
  rm -rf "$TEMP"
  [[ $status -ne 0 ]] || log "cleanup verified: no owned processes, containers or volumes ($PROJECT)"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$ARTIFACT_DIR"
[[ -d "$CLIENT/.git" ]] || { log "client checkout missing: $CLIENT"; exit 1; }
[[ -z "$(git -C "$CLIENT" status --porcelain --untracked-files=all | grep -vE '^\?\? (test-results/|dist/)' || true)" ]] || { log "client checkout is dirty; refusing to modify it"; exit 1; }
for port in "$API_PORT" "$CLIENT_PORT" "$POSTGRES_PORT" "$FIXTURE_PORT"; do
  if lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | grep -q .; then log "port is already in use: $port"; exit 1; fi
done

actual_hash=$(shasum -a 256 "$SPEC" | awk '{print $1}')
config_hash=$(shasum -a 256 "$CONFIG" | awk '{print $1}')
printf 'LOADED SPEC path=%s sha256=%s\nLOADED CONFIG path=%s sha256=%s\n' "$SPEC" "$actual_hash" "$CONFIG" "$config_hash" | tee "$ARTIFACT_DIR/source.txt"
cp "$SPEC" "$TEMP/repaired.spec.ts"
node - "$SPEC" "$TEMP/old.spec.ts" <<'NODE'
const fs = require('fs');
const [source, output] = process.argv.slice(2);
let text = fs.readFileSync(source, 'utf8');
const marker = "test('creates lists edits and deletes both settings through real organization HTTP'";
const at = text.indexOf(marker);
if (at < 0) throw new Error('second provider-settings test was not found');
const old = text.slice(at).replaceAll('fixture/settings-github-${suffix}', 'fixture/settings-api-github-${suffix}')
  .replaceAll('fixture/settings-gitlab-${suffix}', 'fixture/settings-api-gitlab-${suffix}');
text = text.slice(0, at) + old;
if (text === fs.readFileSync(source, 'utf8')) throw new Error('old-identity mutation did not change the source');
fs.writeFileSync(output, text);
NODE
old_hash=$(shasum -a 256 "$TEMP/old.spec.ts" | awk '{print $1}')
printf 'DISPOSABLE OLD SPEC path=%s sha256=%s\nINVOCATION=%s\nCLIENT=%s\nPORTS postgres=%s api=%s client=%s fixture=%s\n' \
  "$TEMP/old.spec.ts" "$old_hash" "agent-ops-identity-fixed-$API_PORT" "$CLIENT" "$POSTGRES_PORT" "$API_PORT" "$CLIENT_PORT" "$FIXTURE_PORT" | tee -a "$ARTIFACT_DIR/source.txt"

mkdir -p "$TEMP/client"
cp "$CLIENT/package.json" "$CLIENT/pnpm-lock.yaml" "$TEMP/client/"
ln -s "$CLIENT/node_modules" "$TEMP/client/node_modules"
mkdir -p "$TEMP/client/scripts"
cp "$CONFIG" "$TEMP/client/scripts/smoke.playwright.config.ts"
cp "$TEMP/repaired.spec.ts" "$TEMP/client/scripts/smoke.browser.spec.ts"

ACTIVE_COMPOSE=(env "POSTGRES_PUBLISH=127.0.0.1" "POSTGRES_HOST_PORT=$POSTGRES_PORT" "SMOKE_API_PORT=$API_PORT" "SMOKE_CLIENT_PORT=$CLIENT_PORT" "CLIENT_DIR=$CLIENT" "JWT_SECRET=$JWT_SECRET" "REPOSITORY_ENCRYPTION_KEY=$ENCRYPTION_KEY" "SMOKE_FIXTURE_PORT=$FIXTURE_PORT" "SMOKE_GITLAB_FIXTURE_URL=http://host.docker.internal:$FIXTURE_PORT" "SMOKE_WEBHOOK_SECRET=fixture-webhook-secret" "SMOKE_BACKFILL_TOKEN=fixture-backfill-token" "SMOKE_ORGANIZATION_ID=d0000000-0000-4000-8000-000000000011" "SCM_REPOSITORIES_JSON=[]")

reset_database() {
  compose stop api >/dev/null 2>&1 || true
  compose exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO agent_ops; GRANT ALL ON SCHEMA public TO public' >/dev/null
  compose up -d api >/dev/null
  for _ in $(seq 1 90); do curl --fail --silent "http://127.0.0.1:$API_PORT/readyz" >/dev/null && break; sleep 1; done
  curl --fail --silent "http://127.0.0.1:$API_PORT/readyz" >/dev/null
  compose exec -T postgres psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "INSERT INTO organizations (id,name) VALUES ('d0000000-0000-4000-8000-000000000011','Identity smoke tenant'); INSERT INTO accounts (id,email,password_hash) VALUES ('d0000000-0000-4000-8000-000000000010','smoke@example.invalid','\$2a\$12\$YFnANTVzpBZyF5Nv7KGX9.yAHCBvay6/hL8uKEF336X0dmSMHrfwS'); INSERT INTO organization_memberships (id,organization_id,account_id,role) VALUES ('d0000000-0000-4000-8000-000000000012','d0000000-0000-4000-8000-000000000011','d0000000-0000-4000-8000-000000000010','owner')" >/dev/null
  login=$(curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"email":"smoke@example.invalid","password":"smoke-password-not-a-secret"}' "http://127.0.0.1:$API_PORT/api/v1/auth/login")
  account_token=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' <<<"$login")
  selected=$(curl --fail --silent --show-error -H "Authorization: Bearer $account_token" -H 'Content-Type: application/json' -d '{"organization_id":"d0000000-0000-4000-8000-000000000011"}' "http://127.0.0.1:$API_PORT/api/v1/auth/select-organization")
  SESSION_TOKEN=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' <<<"$selected")
  export SESSION_TOKEN
}

compose up -d postgres
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
compose up -d --build api
for _ in $(seq 1 90); do curl --fail --silent "http://127.0.0.1:$API_PORT/readyz" >/dev/null && break; sleep 1; done
curl --fail --silent "http://127.0.0.1:$API_PORT/readyz" >/dev/null
reset_database
(cd "$CLIENT" && APP_API_URL="http://127.0.0.1:$API_PORT" pnpm build)
(cd "$CLIENT" && APP_API_URL="http://127.0.0.1:$API_PORT" exec pnpm preview --host 127.0.0.1 --port "$CLIENT_PORT") >"$ARTIFACT_DIR/preview.log" 2>&1 &
PREVIEW_PID=$!
for _ in $(seq 1 60); do curl --fail --silent "http://127.0.0.1:$CLIENT_PORT/board" >/dev/null && break; sleep 1; done
curl --fail --silent "http://127.0.0.1:$CLIENT_PORT/board" >/dev/null

run_spec() {
  local source=$1 run=$2 expect_failure=$3
  local loaded=$TEMP/client/scripts/smoke.browser.spec.ts
  local actual
  cp "$source" "$loaded"
  actual=$(shasum -a 256 "$loaded" | awk '{print $1}')
  [[ "$actual" == "$([[ "$run" == control ]] && printf %s "$old_hash" || printf %s "$actual_hash")" ]]
  printf 'RUN=%s loaded_spec=%s sha256=%s invocation=agent-ops-identity-fixed-%s\n' "$run" "$loaded" "$actual" "$API_PORT" | tee -a "$ARTIFACT_DIR/runs.txt"
  set +e
  local play_config=$TEMP/client/scripts/smoke.playwright.config.ts
  local config_loaded_hash
  config_loaded_hash=$(shasum -a 256 "$play_config" | awk '{print $1}')
  printf 'RUN_CONFIG=%s loaded_config=%s sha256=%s\n' "$run" "$play_config" "$config_loaded_hash" | tee -a "$ARTIFACT_DIR/runs.txt"
  [[ "$config_loaded_hash" == "$config_hash" ]] || { log "loaded config differs from recorded source"; return 1; }
  compose exec -T postgres psql -U agent_ops -d agent_ops -At -F '|' -c 'SELECT provider,full_name FROM repositories ORDER BY provider,full_name' >"$ARTIFACT_DIR/$run-before-identities.txt"
  (cd "$TEMP/client" && SMOKE_ARTIFACT_DIR="$ARTIFACT_DIR/$run" SMOKE_CLIENT_URL="http://127.0.0.1:$CLIENT_PORT" APP_API_URL="http://127.0.0.1:$API_PORT" SMOKE_API_URL="http://127.0.0.1:$API_PORT" SESSION_TOKEN="$SESSION_TOKEN" SMOKE_INVOCATION_ID="agent-ops-identity-fixed-$API_PORT" ./node_modules/.bin/playwright test --config="$play_config" --grep='creates lists edits and deletes both settings through real organization HTTP|creates lists edits and deletes both settings through real organization API' --reporter=line) >"$ARTIFACT_DIR/$run.log" 2>&1
  local result=$?
  set -e
  if [[ $expect_failure == yes && $result -eq 0 ]]; then
    log "$run passed; historical expected-201/uniqueness regression is not reproducible with this identical-input runtime"
    return 2
  fi
  if [[ $expect_failure == yes ]]; then
    [[ $result -ne 0 ]] || { log "$run unexpectedly passed"; return 1; }
    grep -Fq 'Expected: 201' "$ARTIFACT_DIR/$run.log" || { log "$run failed without the ordinary expected-201 assertion"; return 1; }
    grep -Fq 'Received: 500' "$ARTIFACT_DIR/$run.log" || { log "$run did not observe HTTP 500"; return 1; }
    compose logs --no-color api >"$ARTIFACT_DIR/server.log" 2>&1
    grep -Fq 'repositories_provider_full_name_key' "$ARTIFACT_DIR/server.log" || { log "server log lacks observed repositories_provider_full_name_key"; return 1; }
    log "EXPECTED OLD-IDENTITY CONTROL: browser expected-201 assertion observed HTTP 500; real server uniqueness diagnostic captured"
    return 0
  fi
  [[ $result -eq 0 ]] || { log "$run Playwright run failed (exit $result)"; return 1; }
  log "$run browser identity test passed"
}

reset_database
run_spec "$TEMP/repaired.spec.ts" baseline no
control_status=0
reset_database
run_spec "$TEMP/old.spec.ts" control yes || control_status=$?
compose exec -T postgres psql -U agent_ops -d agent_ops -At -F '|' -c "SELECT provider,full_name FROM repositories WHERE full_name LIKE 'fixture/settings-%-agent-ops-identity-fixed-$API_PORT' ORDER BY provider,full_name" >"$ARTIFACT_DIR/persisted-identities.txt"
if [[ $control_status -eq 2 ]]; then
  printf 'CONTROL=PASS (historical failure not reproduced)\n' >"$ARTIFACT_DIR/control-result.txt"
elif [[ $control_status -ne 0 ]]; then
  exit "$control_status"
else
  printf 'CONTROL=OBSERVED_EXPECTED_FAILURE\n' >"$ARTIFACT_DIR/control-result.txt"
fi
compose exec -T postgres psql -U agent_ops -d agent_ops -At -c "SELECT conname,conrelid::regclass FROM pg_constraint WHERE conname='repositories_provider_full_name_key'" >"$ARTIFACT_DIR/unique-constraint.txt"
if [[ -f "$ARTIFACT_DIR/server.log" ]]; then
  if grep -F 'repositories_provider_full_name_key' "$ARTIFACT_DIR/server.log" >"$ARTIFACT_DIR/uniqueness-diagnostic.log"; then :; fi
else
  printf 'No uniqueness diagnostic observed during the old-identity browser run.\n' >"$ARTIFACT_DIR/uniqueness-diagnostic.log"
fi
compose logs --no-color api >"$ARTIFACT_DIR/server.log" 2>&1
printf 'Historical identity mutation changes only the second test identity prefix from settings-github/gitlab to settings-api-github/gitlab.\n' >"$ARTIFACT_DIR/source-comparison.txt"
sed -n '98,115p' "$TEMP/old.spec.ts" >"$ARTIFACT_DIR/old-source-identities.txt"
sed -n '98,115p' "$TEMP/repaired.spec.ts" >"$ARTIFACT_DIR/repaired-source-identities.txt"
reset_database
run_spec "$TEMP/repaired.spec.ts" restored no
compose exec -T postgres psql -U agent_ops -d agent_ops -At -F '|' -c "SELECT provider,full_name FROM repositories WHERE full_name LIKE 'fixture/settings-%-agent-ops-identity-fixed-$API_PORT' ORDER BY provider,full_name" >"$TEMP/restored-identities.txt"
[[ ! -s "$TEMP/restored-identities.txt" ]] || { log "restored run left unexpected repository identities"; exit 1; }
printf 'IDENTITY BASELINE PASSED\nOLD-IDENTITY CONTROL RESULT: %s\nIDENTITY RESTORATION PASSED\n' "$(<"$ARTIFACT_DIR/control-result.txt")" | tee "$ARTIFACT_DIR/result.txt"
