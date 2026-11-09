# Agent Ops

Agent Ops turns agent work into evidence you can act on. It answers three questions that GitLab and GitHub do not:

- What did AI work cost? See cost by week, merge request, issue, tool, and model.
- Is it getting more efficient? See cache-hit rate, cost per merged MR, rework rate, and first-pass CI rate.
- What needs attention now? See work stuck in CI or review, sessions with no MR, and models without a price.

GitLab and GitHub remain the source for issue and merge-request detail; Agent Ops links to them instead of copying them.

## Start in five steps

The shortest supported local path is: start the isolated Compose stack, create the first account, connect a GitLab project,
push one synthetic transcript, then open **Overview**. The repository does not promise a five-minute duration: record the
elapsed time on your machine. The complete, command-by-command guide is [docs/install.md](docs/install.md).

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

Then follow [the timed quickstart](docs/install.md#timed-five-step-quickstart). Use only synthetic credentials and an
isolated Compose project, ports, and volume. Do not use this local setup as a production deployment recipe.

## What is here

- `api/` — Go API, PostgreSQL persistence, provider webhooks, account bootstrap, and transcript ingestion.
- `docs/install.md` — local startup, first account, GitLab project, transcript, Overview, and data-safety guidance.
- `docs/` — architecture and operational notes.

The client is a separate repository. The current documentation describes only capabilities present in this tree; pending
W18 UI/API work is not presented as available.

## Development

API prerequisites are Docker Compose and Go. Read [docs/be-standards.md](docs/be-standards.md) before changing API code.
For the full local acceptance smoke, see the [client smoke instructions](docs/install.md#browser-result).
