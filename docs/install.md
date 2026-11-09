# Local installation and adoption

This is the adoption entry point for the current tree. It describes a persistent
local/development Compose stack. The production-shaped local stack below is a
separate option; it is not a hosted deployment or fresh-machine onboarding.

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
export REPOSITORY_ENCRYPTION_KEY="$(openssl rand -hex 16)"
export AGENT_RUN_TOKEN='local-agent-run-token'
docker compose config --quiet
docker compose up --build -d
```

`CLIENT_DIR`, `JWT_SECRET`, and `REPOSITORY_ENCRYPTION_KEY` are required by the
current `docker-compose.yml`. `AGENT_RUN_TOKEN` is optional: leave it empty to
disable transcript ingestion, or set the same private value used by the CLI to
enable it. The API loader permits an empty encryption key for
deployments that do not configure encrypted repository secrets, but any supplied
key must be exactly 32 bytes. The default host ports are client `4173`, API
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
Sign in at `http://localhost:${SMOKE_CLIENT_PORT:-4173}/login`. If you change
`AGENT_RUN_TOKEN`, recreate the API container (`docker compose up -d --force-recreate api`)
before pushing transcripts so the API and CLI use the matching token.

## First account and organization

The API has no public account-creation or password-reset endpoint. For a fresh
database, create the first account from the running API container as a local
operator. The command reads the password from stdin and never accepts it as a
flag:

```sh
printf '%s\n' "$AGENTOPS_PASSWORD" | docker compose exec -T api \
  /bootstrap-account --email "$AGENTOPS_EMAIL"
```

For production Compose, use the same command with `-f compose.prod.yml`:

```sh
printf '%s\n' "$AGENTOPS_PASSWORD" | docker compose -f compose.prod.yml exec -T api \
  /bootstrap-account --email "$AGENTOPS_EMAIL"
```

Keep the password out of shell history where possible. These commands require
the stack to be running and do not seed accounts automatically. Do not claim
that a fresh empty database can self-register through the web UI.

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

The dashboard sign-in page is `/login`.

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

Build the CLI from `api/`, set `AGENT_RUN_TOKEN` to the same value configured for
the API before starting the stack, and supply the repository UUID,
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

## Production Compose (local)

This stack keeps PostgreSQL data in the named `agent_ops_production_postgres`
volume, binds published ports to loopback, and runs the API and client as
non-root containers. It expects the client checkout at the merged container
revision, plus secrets supplied through the environment:

```sh
export CLIENT_DIR="$(cd ../agent-ops-client && pwd)"
export POSTGRES_PASSWORD="$(openssl rand -hex 24)"
export JWT_SECRET="$(openssl rand -hex 32)"
export REPOSITORY_ENCRYPTION_KEY="$(openssl rand -hex 16)"
export AGENT_RUN_TOKEN='local-agent-run-token'
docker compose -f compose.prod.yml config --quiet
docker compose -f compose.prod.yml up --build -d
```

`AGENT_RUN_TOKEN` is optional for production-shaped local Compose: without it,
agent-run ingestion is disabled. When it is set, configure the CLI with the
same value. If it changes, recreate the API container before using the CLI:
`docker compose -f compose.prod.yml up -d --force-recreate api`.

The defaults publish the client on `18081`, the API on `18080`, and PostgreSQL
on `15432`; override `PRODUCTION_CLIENT_PORT`, `PRODUCTION_API_PORT`, and
`POSTGRES_HOST_PORT` for free loopback ports. The client routes `/api/` to
`http://api:8080` and preserves the API path and websocket upgrade headers.
Stop without deleting data with `docker compose -f compose.prod.yml down`.
