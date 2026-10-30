# Local installation and adoption

This is the adoption entry point for the current tree. It describes a persistent
local/development Compose stack, not a production deployment. Production Compose
and clean-machine acceptance are separate deliverables.

## Prerequisites

Install these versions or newer:

- Docker Engine with the Docker Compose v2 plugin (`docker compose version` must
  work). Compose uses PostgreSQL 17 from `postgres:17-alpine`.
- Go 1.26.10 or newer when building or running the API/CLI outside containers.
- Node.js 20.19 or newer and pnpm 11.21 or newer when working on the sibling
  client checkout. The local client image installs its dependencies with pnpm.
- `curl`, `jq`, `openssl`, and Git. `pg_dump`, `pg_restore`, and `psql` are needed
  only for the backup/restore commands in [backup-restore.md](backup-restore.md).

## Clone the repositories

Keep the API repository and its client sibling adjacent. The Compose build uses
the client source through `CLIENT_DIR`.

```sh
git clone https://github.com/0Hoag/agent-ops.git
git clone https://github.com/0Hoag/agent-ops-client.git
cd agent-ops
export CLIENT_DIR="$(cd ../agent-ops-client && pwd)"
```

## Start the persistent local stack

Generate local-only values in shell variables. These commands do not print the
secret values. Do not reuse these development values for a hosted deployment.

```sh
export CLIENT_DIR="$(cd ../agent-ops-client && pwd)"
export JWT_SECRET="$(openssl rand -hex 32)"
export REPOSITORY_ENCRYPTION_KEY="$(openssl rand -hex 32)"
docker compose config --quiet
docker compose up --build -d
```

`CLIENT_DIR`, `JWT_SECRET`, and `REPOSITORY_ENCRYPTION_KEY` are required by the
current `docker-compose.yml`. The default host ports are client `4173`, API
`18080`, and PostgreSQL `5432`; override them with `SMOKE_CLIENT_PORT`,
`SMOKE_API_PORT`, and `POSTGRES_HOST_PORT` if needed. The named
`agent_ops_postgres` volume is persistent. Stop the stack without deleting data:

```sh
docker compose down
```

Check liveness and readiness at `http://localhost:${SMOKE_CLIENT_PORT:-4173}/healthz`
and `http://localhost:${SMOKE_CLIENT_PORT:-4173}/readyz`. Direct API checks are
available at `http://localhost:${SMOKE_API_PORT:-18080}/healthz` and `/readyz`.
Open `http://localhost:${SMOKE_CLIENT_PORT:-4173}` for the dashboard.

## First account and organization

The current API has no public account-creation or password-reset endpoint. A
local operator must therefore use an account provisioned by the supported
development/bootstrap process for the environment, then use the API interfaces
below. Do not claim that a fresh empty database can self-register through the
web UI.

With an existing account, log in and keep the returned token out of shell
history and logs:

```sh
API="http://localhost:${SMOKE_API_PORT:-18080}"
read -r -s AGENTOPS_PASSWORD
export AGENTOPS_PASSWORD
LOGIN_JSON=$(curl -fsS "$API/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  --data "$(printf '{\"email\":\"%s\",\"password\":\"%s\"}' "$AGENTOPS_EMAIL" "$AGENTOPS_PASSWORD")")
TOKEN=$(printf '%s' "$LOGIN_JSON" | jq -r '.token')
curl -fsS "$API/api/v1/organizations" -H "Authorization: Bearer $TOKEN"
```

Create the first organization through `POST /api/v1/organizations` with
`{"name":"Your organization"}`. The creator becomes its owner. Select it with
`POST /api/v1/auth/select-organization` and
`{"organization_id":"<uuid>"}`; use the returned token for organization-scoped
requests. The dashboard can then use the same session flow. Organization
membership and invitations are documented in [the organization module
README](../api/modules/organization/README.md).

## Connect GitHub or GitLab

Configure repositories with `SCM_REPOSITORIES_JSON` and provider-specific token
and webhook-secret environment variables. Follow [Provider
configuration](providers.md) for the exact JSON fields, self-hosted base URLs,
and signature headers. The current local API stores repository settings through
the authenticated repository routes; the configured repository must have an
organization ID when it is first synchronized.

Register the provider webhook at `POST /api/v1/webhooks/github` or
`POST /api/v1/webhooks/gitlab` using the provider's signed request. GitHub must
send `X-Hub-Signature-256`; GitLab must send `X-Gitlab-Token`. Use a loopback
reachable URL for local testing or a tunnel you control; this guide does not
perform external webhook registration.

## Push or watch transcripts

Build the CLI from `api/`, set `AGENT_RUN_TOKEN`, and supply the repository UUID,
wave, and worker metadata. `push` accepts one Codex, OpenCode, or Claude JSONL
transcript and `watch` polls Claude's projects directory. The complete supported
flags are in [api/cmd/agentops/README.md](../api/cmd/agentops/README.md).

```sh
cd api
go build -o bin/agentops ./cmd/agentops
export AGENT_RUN_TOKEN='local-agent-run-token'
./bin/agentops push --harness codex --endpoint "$API/api/v1/agent-runs" \
  --repository-id '<repository-uuid>' --wave W-local --worker codex-worker transcript.jsonl
./bin/agentops push --harness opencode --endpoint "$API/api/v1/agent-runs" \
  --repository-id '<repository-uuid>' --wave W-local --worker opencode-worker transcript.jsonl
./bin/agentops push --harness claude --endpoint "$API/api/v1/agent-runs" \
  --repository-id '<repository-uuid>' --wave W-local --worker claude-worker transcript.jsonl
./bin/agentops watch --root "$HOME/.claude/projects" \
  --checkpoint "$HOME/.cache/agentops-claude-watch.json" \
  --endpoint "$API/api/v1/agent-runs" --repository-id '<repository-uuid>' \
  --wave W-local --worker claude-watch --interval 1s
```

For launcher integration, see [usage-reporting.md](usage-reporting.md).

## Upgrade, backup, and rollback

Before changing the checkout or restarting a persistent stack, make a custom
format backup and verify it in an isolated target. Use the exact commands and
safety checks in [backup-restore.md](backup-restore.md). Review the local release
tooling in [release.md](release.md) before selecting a revision. The API image
details are in [api-container.md](api-container.md).

The safe local rollback is: stop the stack, retain the backup, check out the
previous known-good revision, rebuild, and restore only into a new empty database
or disposable Compose project. Never restore over the persistent database and do
not run `docker compose down -v` unless intentionally deleting local data.

The API container contract is linked from [api-container.md](api-container.md);
this guide does not duplicate it.
