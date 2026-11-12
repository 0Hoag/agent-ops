# Reproduction commands

All credentials below are synthetic. Run from this worktree root. Never use an owner database.

```sh
export BUILDX_CONFIG=/tmp/w18r1-api-501-buildx
export JWT_SECRET=synthetic-review-jwt-secret-501
export REPOSITORY_ENCRYPTION_KEY=12345678901234567890123456789012
export AGENT_RUN_TOKEN=synthetic-ingestion-501
export SMOKE_API_PORT=35081 POSTGRES_HOST_PORT=35433
mkdir -p .agent-runs/W18R1-api-501
cp scripts/review-api-501/{worker.sh,compose.review.yml} .agent-runs/W18R1-api-501/
# Verify all three ports are free and the owned project has no container/volume before first startup.
lsof -nP -iTCP:35081 -iTCP:35433 -iTCP:35082 -sTCP:LISTEN
docker ps -a --filter label=com.docker.compose.project=w18r1-api-501
docker volume ls --filter label=com.docker.compose.project=w18r1-api-501
mkdir -p "$BUILDX_CONFIG"
python3 scripts/review-api-501/provider.py > .agent-runs/W18R1-api-501/provider.log 2>&1 &
# Retain that exact PID; stop only that owned process during cleanup.
scripts/heavy.sh docker compose -p w18r1-api-501 -f docker-compose.yml -f .agent-runs/W18R1-api-501/compose.review.yml up --build -d postgres api
curl -fsS http://127.0.0.1:35081/readyz
scripts/heavy.sh sh -c 'for email in review501@example.invalid other501@example.invalid member501@example.invalid; do printf "%s\n" synthetic-review-password | docker compose -p w18r1-api-501 exec -T api /bootstrap-account --email "$email" || exit; done'
python3 scripts/review-api-501/setup.py
export SCM_REPOSITORIES_JSON='[{"provider":"gitlab","full_name":"review/synthetic","webhook_secret_env":"REVIEW_WEBHOOK_SECRET"}]'
scripts/heavy.sh docker compose -p w18r1-api-501 -f docker-compose.yml -f .agent-runs/W18R1-api-501/compose.review.yml up -d api
# Sync via HTTP after readiness: POST /api/v1/repositories/<state.r>/sync with state.a as Bearer.
# Required to populate the synthetic Work/detail record before the sweep.
python3 scripts/review-api-501/sweep.py
python3 scripts/review-api-501/supplement.py
# Optional second webhook provider: create a local GitHub fixture without a backfill token,
# add it to this API configuration, then exercise HMAC authentication:
python3 - <<'PYSETUP'
import sys,json
sys.path.insert(0,"scripts/review-api-501")
from http_common import call,OUT
state=json.load(open(OUT/"state.json"))
call("POST","/api/v1/repositories",{"provider":"github","full_name":"review/github-fixture","gitlab_base_url":"http://host.docker.internal:35082","webhook_secret":"review-secret"},state["a"])
PYSETUP
export SCM_REPOSITORIES_JSON='[{"provider":"gitlab","full_name":"review/synthetic","webhook_secret_env":"REVIEW_WEBHOOK_SECRET"},{"provider":"github","full_name":"review/github-fixture","webhook_secret_env":"REVIEW_WEBHOOK_SECRET"}]'
scripts/heavy.sh docker compose -p w18r1-api-501 -f docker-compose.yml -f .agent-runs/W18R1-api-501/compose.review.yml up -d api
# Wait for /readyz before the next request.
python3 scripts/review-api-501/github_auth.py
python3 scripts/review-api-501/report.py
cd api
GOPATH=/tmp/w18r1-api-501-gopath GOCACHE=/tmp/w18r1-api-501-gocache GOMODCACHE=/tmp/w18r1-api-501-modcache GOLANGCI_LINT_CACHE=/tmp/w18r1-api-501-lintcache ../scripts/heavy.sh make ci
cd ..
scripts/heavy.sh docker compose -p w18r1-api-501 -f docker-compose.yml -f .agent-runs/W18R1-api-501/compose.review.yml down -v --rmi local
# Verify both container and volume label queries return empty; inspect network as well.
```

The Python request records contain exact HTTP method, expanded path/query, JSON/body, status and response. The auth fixtures are in state.json (local synthetic credentials only). Agent ingestion uses X-Agent-Run-Token, webhook uses X-Gitlab-Token plus event/delivery headers; WebSocket cases send a real HTTP Upgrade handshake. GET bodies are ignored by most handlers; the invalid column additionally supplies malformed query fields where applicable. Body-free DELETE/POST actions may legitimately ignore malformed bodies. Wrong-org collection creates target that token's own organization and can succeed; entity reads/updates must not cross tenants. Organization account routes use account membership, independently of selected organization.
