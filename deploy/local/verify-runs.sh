#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CLIENT_DIR="${CLIENT_DIR:?Set CLIENT_DIR to a clean agent-ops-client checkout}"
export AGENT_RUN_MODEL_PRICES='{"gpt-compose-fixture":{"input_per_million":1,"cached_input_per_million":1,"cache_write_per_million":1,"output_per_million":1,"reasoning_per_million":1}}'
export AGENT_RUN_TOKEN=compose-fixture-token
PROJECT="${COMPOSE_PROJECT_NAME:-w3x-runs-$(date +%s)-$$}"
POSTGRES_HOST_PORT="${POSTGRES_HOST_PORT:-$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')}"
SMOKE_API_PORT="${SMOKE_API_PORT:-$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')}"
SMOKE_CLIENT_PORT="${SMOKE_CLIENT_PORT:-$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')}"
ARTIFACT_DIR="${ARTIFACT_DIR:-$ROOT/deploy/local/artifacts/$PROJECT}"
COMPOSE=(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml")

mkdir -p "$ARTIFACT_DIR"
cleanup() {
  local result=$?
  docker rm -f "$PROJECT-api-fixture" >/dev/null 2>&1 || true
  "${COMPOSE[@]}" down --volumes --remove-orphans >"$ARTIFACT_DIR/cleanup.log" 2>&1 || result=1
  if docker ps -aq --filter "name=$PROJECT-api-fixture" | grep -q .; then printf 'owned API fixture container remains\n' >>"$ARTIFACT_DIR/cleanup.log"; result=1; fi
  if "${COMPOSE[@]}" ps -aq | grep -q .; then printf 'owned containers remain\n' >>"$ARTIFACT_DIR/cleanup.log"; result=1; fi
  if docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" | grep -q .; then printf 'owned volumes remain\n' >>"$ARTIFACT_DIR/cleanup.log"; result=1; fi
  for port in "$POSTGRES_HOST_PORT" "$SMOKE_API_PORT" "$SMOKE_CLIENT_PORT"; do
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >"$ARTIFACT_DIR/listener-$port.log" 2>&1; then
      printf 'listener remains on port %s\n' "$port" >>"$ARTIFACT_DIR/cleanup.log"; result=1
    fi
  done
  printf 'containers=absent volumes=absent listeners=absent project=%s\n' "$PROJECT" >>"$ARTIFACT_DIR/cleanup.log"
  exit "$result"
}
trap cleanup EXIT INT TERM

cd "$ROOT"
export CLIENT_DIR POSTGRES_HOST_PORT SMOKE_API_PORT SMOKE_CLIENT_PORT
export AGENT_RUN_MODEL_PRICES AGENT_RUN_TOKEN
"${COMPOSE[@]}" config >"$ARTIFACT_DIR/compose-config.yml"
"${COMPOSE[@]}" up --build -d
"${COMPOSE[@]}" stop api
"${COMPOSE[@]}" rm -f api
"${COMPOSE[@]}" run --build -d --no-deps --name "$PROJECT-api-fixture" -p "127.0.0.1:$SMOKE_API_PORT:8080" -e "AGENT_RUN_MODEL_PRICES=$AGENT_RUN_MODEL_PRICES" -e "AGENT_RUN_TOKEN=$AGENT_RUN_TOKEN" api
for attempt in $(seq 1 45); do
  if curl -fsS "http://127.0.0.1:$SMOKE_API_PORT/readyz" >/dev/null; then break; fi
  if [ "$attempt" = 45 ]; then docker logs "$PROJECT-api-fixture" >"$ARTIFACT_DIR/api-fixture.log"; exit 1; fi
  sleep 1
done
for attempt in $(seq 1 90); do
  if curl -fsS "http://127.0.0.1:$SMOKE_CLIENT_PORT/healthz" >/dev/null; then break; fi
  if [ "$attempt" = 90 ]; then "${COMPOSE[@]}" logs >"$ARTIFACT_DIR/compose.log"; exit 1; fi
  sleep 2
done

docker exec "${PROJECT}-postgres-1" psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO repositories (id, provider, full_name, webhook_secret_env_ref) VALUES
('123e4567-e89b-42d3-a456-426614174000', 'github', 'compose-acceptance/runs', 'unused');
SQL
docker exec "${PROJECT}-postgres-1" psql -U agent_ops -d agent_ops -v ON_ERROR_STOP=1 -c "SELECT id, full_name FROM repositories WHERE id = '123e4567-e89b-42d3-a456-426614174000'" >"$ARTIFACT_DIR/seed-repository-check.txt"
seed_status=$(curl -sS -o "$ARTIFACT_DIR/seed-response.json" -w '%{http_code}' -X POST "http://127.0.0.1:$SMOKE_API_PORT/api/v1/agent-runs" -H 'Content-Type: application/json' -H "X-Agent-Run-Token: $AGENT_RUN_TOKEN" \
  -d '{"repository_id":"123e4567-e89b-42d3-a456-426614174000","wave":"W3x-compose-acceptance","worker":"ComposeBrowserFixture","model":"gpt-compose-fixture","issue_numbers":[237],"input_tokens":23456,"cached_input_tokens":12345,"cache_write_tokens":456,"output_tokens":789,"reasoning_tokens":123,"started_at":"2026-10-25T00:00:00Z","ended_at":"2026-10-25T00:01:00Z"}')
if [ "$seed_status" != 201 ]; then
  docker logs "$PROJECT-api-fixture" >"$ARTIFACT_DIR/api-fixture.log" 2>&1 || true
  printf 'fixture create returned HTTP %s: ' "$seed_status" >&2
  python3 -c 'import pathlib,sys; print(pathlib.Path(sys.argv[1]).read_text())' "$ARTIFACT_DIR/seed-response.json" >&2
  exit 1
fi

curl -fsS -D "$ARTIFACT_DIR/api-headers.txt" "http://127.0.0.1:$SMOKE_CLIENT_PORT/api/api/v1/agent-runs/aggregate" -o "$ARTIFACT_DIR/api-response.json"
node -e 'const a=require(process.argv[1]).data; const x=a.find(r=>r.wave==="W3x-compose-acceptance"); if(!x||x.worker!=="ComposeBrowserFixture"||x.input_tokens!==23456||x.cached_input_tokens!==12345||x.output_tokens!==789||Math.abs(x.cost_usd-0.037169)>0.0000001) process.exitCode=1; console.log(JSON.stringify(x ?? null))' "$ARTIFACT_DIR/api-response.json"

cat >"$ARTIFACT_DIR/assert-runs.mjs" <<'JS'
import assert from 'node:assert/strict';
const origin = process.env.ORIGIN;
const artifactDir = process.env.ARTIFACT_DIR;
const { chromium } = await import(`${process.env.CLIENT_DIR}/node_modules/playwright/index.mjs`);
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  const apiResponses = [];
  page.on('response', (response) => {
    if (response.url().includes('/api/api/v1/agent-runs/aggregate')) apiResponses.push(response);
  });
  await page.goto(`${origin}/runs`);
  try {
    for (const text of ['W3x-compose-acceptance', 'ComposeBrowserFixture', '$0.0372']) {
      await page.getByText(text, { exact: false }).waitFor({ timeout: 10000 });
    }
  } catch (error) {
    await page.screenshot({ path: `${artifactDir}/runs-failure.png`, fullPage: true });
    throw new Error(`RUNS DOM ASSERTION: fixture wave/worker/cost absent at ${origin}/runs`, { cause: error });
  }
  assert(apiResponses.some((response) => response.status() === 200), 'RUNS NETWORK ASSERTION: no HTTP 200 same-origin aggregate request');
  await page.screenshot({ path: `${artifactDir}/runs-baseline.png`, fullPage: true });
  console.log(`RUNS ACCEPTANCE PASS: ${origin}/runs DOM fixture and same-origin HTTP 200 aggregate`);
} finally {
  await browser.close();
}
JS

export ORIGIN="http://127.0.0.1:$SMOKE_CLIENT_PORT" ARTIFACT_DIR
"$CLIENT_DIR/node_modules/.bin/playwright" install chromium
node "$ARTIFACT_DIR/assert-runs.mjs" | tee "$ARTIFACT_DIR/baseline.log"

# Disposable data-path control: bypass only the API proxy while preserving the assertion verbatim.
docker exec "${PROJECT}-client-1" sh -c 'sed -i "s#proxy_pass http://api:8080/;#proxy_pass http://127.0.0.1:9/;#" /etc/nginx/conf.d/default.conf && nginx -s reload'
if node "$ARTIFACT_DIR/assert-runs.mjs" >"$ARTIFACT_DIR/control.log" 2>&1; then
  printf 'control unexpectedly passed\n' >&2
  exit 1
fi
grep -F 'RUNS DOM ASSERTION: fixture wave/worker/cost absent' "$ARTIFACT_DIR/control.log"
printf 'BROKEN-PROXY CONTROL PASS: unchanged assertion failed with intended diagnostic\n' | tee -a "$ARTIFACT_DIR/control.log"
printf 'Source nginx.conf unchanged; disposable proxy mutation existed only in owned container.\n' >"$ARTIFACT_DIR/mutation-restored.log"
docker exec "${PROJECT}-client-1" sh -c 'sed -i "s#proxy_pass http://127.0.0.1:9/;#proxy_pass http://api:8080/;#" /etc/nginx/conf.d/default.conf && nginx -s reload'
node "$ARTIFACT_DIR/assert-runs.mjs" >"$ARTIFACT_DIR/restored.log"
printf 'Restored proxy recheck: '; cat "$ARTIFACT_DIR/restored.log"
printf 'Artifacts: %s\n' "$ARTIFACT_DIR"
