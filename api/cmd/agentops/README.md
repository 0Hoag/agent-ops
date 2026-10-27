# agentops watch

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
