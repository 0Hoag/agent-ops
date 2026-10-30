# OpenCode MCP integration

This proof runs the real `agentops mcp` server against a disposable PostgreSQL database and API process, then lets one OpenCode session discover and call all four tools: `list_issues`, `wave_status`, `my_costs`, and `similar_failures`.

## Prerequisites

Install Docker Compose, Go, `curl`, `jq`, `openssl`, Python 3, and OpenCode with an authenticated model provider. Run from the repository root. The script chooses loopback ports and creates only a project-scoped Compose fixture. OpenCode uses disposable XDG directories beneath the canonical artifact directory; any private auth copy is removed during cleanup.

## Run

```sh
bash -n scripts/smoke-mcp.sh
scripts/heavy.sh bash scripts/smoke-mcp.sh
```

The script seeds an organization, repository, issue, wave, agent-run row, and local failure note. It writes redacted evidence to `/Users/zeross/agent-ops/.agent-runs/W15f-artifacts/`: `session.jsonl`, `summary.txt`, `control.txt`, `source-sha.txt`, and `cleanup.txt`. The disposable JWT, API binary, and private OpenCode runtime copy are removed by the exit trap and are never committed.

The OpenCode config is generated under that evidence directory and is not the root `opencode.json`. The session is instructed to use MCP only; hand-written JSON-RPC calls are not a substitute for this run.

## Failing control and cleanup

The restored assertions require four successful structured tool results, one each for `list_issues`, `wave_status`, `my_costs`, and `similar_failures`, with their known seeded values. Prompt-only strings and failed tool results are regression fixtures and must exit non-zero. The exit trap stops the owned API process, removes only the uniquely named Compose project, deletes its volumes, removes private runtime credentials, and records that no owned containers or volumes remain.

To remove an interrupted fixture, use only its recorded project name:

```sh
docker compose -p agentops-mcp-<uid>-<pid> -f docker-compose.yml down --volumes --remove-orphans
```

Do not paste tokens into shell history or committed files. If OpenCode or the API reports a protocol defect, preserve the diagnostic in the evidence directory and report the exact reproduction instead of changing MCP product code.
