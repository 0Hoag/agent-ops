# Claude transcript importer

This small W3d CLI reads a Claude Code JSONL transcript from an explicit `-path` (files copied from
`~/.claude/projects` are a normal input) and submits one normalized agent run per transcript session.
Use synthetic fixtures for tests; the importer never prints transcript content or the agent-run token.

Streaming records are deduplicated by session metadata and message UUID. Input, cache-read,
cache-write, output, and reasoning counters remain separate. A caller-selected `-checkpoint` stores
only successful session identities and is updated after each successful POST. A failed POST leaves the
session retryable. This is importer-local durable deduplication, not a cross-machine or crash-exactly-once
guarantee. Because the current API is append-only, a changed/revised existing session is not silently
appended: use a new checkpoint only when intentionally importing a distinct session.

Example:

```bash
AGENT_RUN_TOKEN=secret go run ./cmd/import-claude \
  -path ./cmd/import-claude/testdata/synthetic.jsonl \
  -checkpoint /tmp/claude-import-checkpoint.json \
  -repository-id 00000000-0000-0000-0000-000000000001 \
  -endpoint http://127.0.0.1:8080/api/v1/agent-runs
```
