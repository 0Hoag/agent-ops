# Local five-step quickstart

This guide is for a disposable local/development stack. It answers the product questions from
[ux-principles.md](ux-principles.md): cost, efficiency, and current attention. It does not describe a hosted deployment.

## Prerequisites

Docker with Compose v2, Git, `openssl`, `curl`, and `jq` are required. Go is required to build the transcript CLI.
The client checkout is required by the Compose client image.

## Timed five-step quickstart

Start a timer immediately before the first command and record every command and result. The example uses an isolated
Compose project, named volume, and loopback ports; replace no shared or persistent owner stack.

### 1. Clone and start Compose

```sh
git clone https://github.com/0Hoag/agent-ops.git
git clone https://github.com/0Hoag/agent-ops-client.git
cd agent-ops
export CLIENT_DIR="$(cd ../agent-ops-client && pwd)"
export SMOKE_API_PORT=28080 SMOKE_CLIENT_PORT=24173 POSTGRES_HOST_PORT=25432
export JWT_SECRET="$(openssl rand -hex 32)"
export REPOSITORY_ENCRYPTION_KEY="$(openssl rand -hex 32)"
export AGENT_RUN_TOKEN="local-only-transcript-token"
docker compose -p agent-ops-docs config --quiet
docker compose -p agent-ops-docs up --build -d
```

`JWT_SECRET` and `REPOSITORY_ENCRYPTION_KEY` are required by Compose. The encryption key must be exactly 32 bytes; the
`openssl rand -hex 32` command supplies 64 hexadecimal characters (32 bytes). `AGENT_RUN_TOKEN` is the private token
accepted by transcript ingestion. `CLIENT_DIR` supplies the separate client source to the client image. The port variables
only change the host bindings. Each variable is consumed by `docker-compose.yml` and is deliberately local to this run.

For a responsive local setup, Docker Desktop is recommended at 4 CPUs. `nice` lowers the priority of the host command;
it does not limit the CPU available to Docker Desktop's VM.

Check both services before continuing:

```sh
curl -fsS "http://127.0.0.1:${SMOKE_API_PORT}/healthz"
curl -fsS "http://127.0.0.1:${SMOKE_API_PORT}/readyz"
curl -fsS "http://127.0.0.1:${SMOKE_CLIENT_PORT}/healthz"
```

### 2. Create the first account

Use synthetic local values. Bootstrap reads the password from stdin; it is never a command-line argument.

```sh
export AGENTOPS_EMAIL='docs-smoke@example.invalid'
export AGENTOPS_PASSWORD='synthetic-docs-password'
printf '%s\n' "$AGENTOPS_PASSWORD" | docker compose -p agent-ops-docs exec -T api \
  /bootstrap-account --email "$AGENTOPS_EMAIL"
```

Log in without printing the password or token:

```sh
export API="http://127.0.0.1:${SMOKE_API_PORT}"
LOGIN_JSON=$(curl -fsS "$API/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data "$(jq -cn --arg e "$AGENTOPS_EMAIL" --arg p "$AGENTOPS_PASSWORD" '{email:$e,password:$p}')")
export TOKEN=$(printf '%s' "$LOGIN_JSON" | jq -r '.token')
```

Create and select the first organization. The returned organization token is used for repository settings and Overview.

```sh
ORG_JSON=$(curl -fsS "$API/api/v1/organizations" -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' --data '{"name":"Docs smoke"}')
export ORG_ID=$(printf '%s' "$ORG_JSON" | jq -r '.data.id')
SELECT_JSON=$(curl -fsS "$API/api/v1/auth/select-organization" -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' --data "$(jq -cn --arg id "$ORG_ID" '{organization_id:$id}')")
export TOKEN=$(printf '%s' "$SELECT_JSON" | jq -r '.token')
```

### 3. Connect one GitLab project

This path stores the GitLab project settings through the authenticated repository endpoint. It does not contact GitLab,
and synthetic webhook/backfill values are not GitLab acceptance credentials. For a real project, use its
`group/project` path and real credentials held securely outside shell history.

```sh
REPO_JSON=$(curl -fsS "$API/api/v1/repositories" -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' --data '{"provider":"gitlab","full_name":"docs/synthetic-project","gitlab_base_url":"https://gitlab.com","webhook_secret":"synthetic-webhook-secret","backfill_token":"synthetic-backfill-token"}')
export REPOSITORY_ID=$(printf '%s' "$REPO_JSON" | jq -r '.data.id')
printf '%s\n' "$REPO_JSON" | jq '{provider:.data.provider,full_name:.data.full_name,id:.data.id,webhook_url:.data.webhook_url}'
```

The API encrypts the two supplied secrets with `REPOSITORY_ENCRYPTION_KEY` and returns them only on creation. Do not paste
the complete response into logs. Configure a real GitLab webhook separately at the returned `webhook_url`; that external
registration is not part of this timed local run.

### 4. Push one transcript

Create a synthetic JSONL transcript and push it with the private token. This proves local ingestion only, not a real GitLab
delivery or provider synchronization.

```sh
cd api
go build -o /tmp/agentops-docs ./cmd/agentops
printf '%s\n' '{"type":"turn.completed","session_id":"docs-synthetic-session","timestamp":"2026-11-09T00:00:00Z","model":"synthetic-model","usage":{"input_tokens":10,"output_tokens":5}}' >/tmp/agentops-docs.jsonl
AGENT_RUN_TOKEN="$AGENT_RUN_TOKEN" /tmp/agentops-docs push --harness codex --endpoint "$API/api/v1/agent-runs" \
  --repository-id "$REPOSITORY_ID" --wave docs-smoke --worker synthetic-worker /tmp/agentops-docs.jsonl
cd ..
```

### 5. Open Overview

Open `http://127.0.0.1:${SMOKE_CLIENT_PORT}` in a browser, sign in at `/login`, select **Overview**, and capture a
screen shot showing the synthetic run or the documented empty state. The Overview result is the acceptance artifact for
this run. Do not call a pending W18 dashboard/API screen complete if this route is unavailable in the checked-out client.

## Browser result

Attach a browser screenshot at desktop width and at 390 px width, plus the timed command log. If Docker, the browser, or
real GitLab credentials prevent a step, record the exact failing command, mark the result unverified, and stop; never retry
against a shared stack or claim real GitLab acceptance from synthetic data.

## First-run acceptance checklist

Record this checklist in the acceptance evidence for each fresh clone. Replace every bracketed value with the observed
result; do not write `pass` for a command that was not run.

```text
Run started (UTC): [timestamp]
Total elapsed seconds: [measured seconds] (target: under 600; this guide makes no duration promise)

[ ] Clone agent-ops — exit [0/non-zero], [seconds]
[ ] Clone agent-ops-client — exit [0/non-zero], [seconds]
[ ] Compose config — exit [0/non-zero], [seconds]
[ ] Compose up --build -d — exit [0/non-zero], [seconds]
[ ] /healthz — exit [0/non-zero], [seconds]
[ ] /readyz — exit [0/non-zero], [seconds]
[ ] Client /healthz — exit [0/non-zero], [seconds]
[ ] Bootstrap account — exit [0/non-zero], [seconds]
[ ] Login and select organization — exit [0/non-zero], [seconds]
[ ] Create GitLab project settings — exit [0/non-zero], [seconds]
[ ] Build and push transcript — exit [0/non-zero], [seconds]
[ ] Overview populated with the synthetic run — observed [yes/no/unverified]
[ ] First-run checklist screenshot at 1280 px — [path or not captured]
[ ] Overview screenshot at 1280 px — [path or not captured]
[ ] Overview screenshot at 390 px — [path or not captured]
```

The checklist is evidence, not a substitute for executing the flow. A blocked command leaves subsequent items
unverified, and the exact command, output, and blocker must remain in the timed log.

## Stop and preserve data

Stop this isolated stack without deleting its data while investigating:

```sh
docker compose -p agent-ops-docs down
```

Delete only this disposable project and volume after archiving evidence:

```sh
docker compose -p agent-ops-docs down -v
```

Never run `down -v` against an owner or persistent project. For backups and rollback, use [backup-restore.md](backup-restore.md).
