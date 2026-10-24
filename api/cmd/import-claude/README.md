# Claude transcript importer

This small W3d CLI reads a Claude Code JSONL transcript from an explicit `-path` (files copied from
`~/.claude/projects` are a normal input) and submits one normalized agent run per transcript session.
Use synthetic fixtures for tests; the importer never prints transcript content or the agent-run token.

Streaming records are deduplicated within each transcript by session metadata and message UUID. Input, cache-read,
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

## Real persisted-repeat acceptance

Run `./persisted-repeat.sh` from this directory. It builds the importer and API, starts a uniquely named
PostgreSQL fixture with a dynamically selected host port other than 5432, seeds one repository, then runs
the compiled importer twice as separate processes against the same checkpoint and database. It queries the
actual `agent_runs` rows after each invocation and asserts the exact count and token sums are unchanged on
the second run. The script removes only its own compose project and verifies its containers, volumes, and
API listener are absent on exit.
