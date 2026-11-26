# agentops watch

For a real local OpenCode proof using all four tools, see [MCP integration](../../../docs/mcp.md).

## agentops mcp

`agentops mcp` serves the MCP JSON-RPC protocol over stdin/stdout. It exposes `list_issues`, `wave_status`, `my_costs`, and `similar_failures` through the authenticated API.

```bash
AGENTOPS_API_URL=http://127.0.0.1:8080 AGENTOPS_API_TOKEN=selected-organization-jwt \
  go run ./cmd/agentops mcp
```

`my_costs` reads the authenticated `/api/v1/agent-runs/aggregate` data and accepts optional `repository_id`, `wave`, and `issue_number` filters. `similar_failures` accepts `query` and `repository_id`; it searches the configured local `FIVE-MINUTES.md` (default path `FIVE-MINUTES.md`, configurable with `--five-minutes` or `AGENTOPS_FIVE_MINUTES_PATH`) and authenticated insights evidence. It never accepts a file path from a tool call.

The token is sent only as an `Authorization: Bearer` header to the configured API URL and is never written to stdout.

`agentops watch` recursively polls a Claude projects directory for `.jsonl` transcripts and submits changed
Claude sessions through the same authenticated agent-run API used by `push`.

```bash
AGENT_RUN_TOKEN=secret go run ./cmd/agentops watch \
  --root ~/.claude/projects \
  --checkpoint /tmp/agentops-claude-watch.json \
  --repository-id 00000000-0000-0000-0000-000000000001 \
  --wave W9 --worker claude-watch
```

The checkpoint contains file size/mtime and SHA-256 digests of normalized session metadata only. It is written
atomically after all changed files submit successfully, so an incomplete trailing JSONL record remains retryable
and a restarted watcher resubmits no unchanged session. The API upsert preserves one cumulative row per
repository, harness, and session ID. Use synthetic fixtures for tests; do not inspect private Claude transcripts.

`agentops hook` reads a Claude `Stop` hook event from stdin, validates its `session_id`, `transcript_path`, and
`cwd`, and submits only that session. It uses the same `AGENT_RUN_*` variables as `push`; failures are written to
stderr and return a non-zero status.

## Claude Code setup

For sessions started anywhere, including `~` outside a Git checkout, install a user-level hook:

```bash
agentops setup claude --global [--endpoint <url>]
```

This writes `~/.claude/settings.json`. The hook reads the active session and transcript path supplied by Claude Code,
then records local usage under the server's configured organization. It does not require a repository or infer an MR.
The API uses `AGENT_RUN_ORGANIZATION_ID` when set; if unset, it safely selects the only organization in the database.
With zero or multiple organizations, configure that server variable. The client never submits an organization ID.
Set `AGENT_RUN_TOKEN` in the environment inherited by Claude Code. The token is not stored in settings.

For repository-bound ingestion, run setup from that checkout and pass its repository UUID:

```bash
agentops setup claude --repository-id <repository-id> --wave <wave> --worker <worker> [--endpoint <url>]
```

This writes `.claude/settings.local.json`. Existing settings and hooks are preserved; repeat setup is safe. Remove
only the Agent Ops hook using `agentops setup claude --global --uninstall` or run the same command from a checkout
without `--global` to remove the repo-local hook.
