# W21b transcript reconciliation (#528)

This verifier independently recomputes counters from three real, direct Claude Code session JSONL files. It streams one line at a time and decodes only type, identity, parent-tool marker, UUID, and usage counters; prompt, text, and tool payloads are discarded. It selects the three newest direct JSONL candidates under `~/.claude/projects` by filesystem metadata and reports only opaque aliases.

Top-level assistant records with a session ID and UUID are candidates. Records with `parent_tool_use_id` are sidechains and are excluded. Repeated UUIDs are retries/streamed representations and are counted once; the duplicate count is reported. Input, cache-read, cache-write, output, and reasoning remain separate counters.

## Exact commands and evidence

```text
bash scripts/verify/transcript-reconcile-528.sh --self-test
go test ./scripts/verify/transcript-reconcile-528.go
gofmt -d scripts/verify/transcript-reconcile-528.go
bash -n scripts/verify/transcript-reconcile-528.sh
bash scripts/verify/transcript-reconcile-528.sh
git diff --check
```

The self-test embeds synthetic records and must pass a deliberately wrong retry-count rule using identical input. Synthetic proof is not real-page acceptance.

## Real comparison

The verifier reports the independent three-session counters. Persisted/API/page comparison is **BLOCKED and unaccepted**: listener ownership, authentication, persisted linkage, and an isolated fixture were not established. No existing records were imported or mutated.

## Privacy and scope

No transcript content, absolute filename, credential, user message, or payload is printed, copied, committed, or uploaded. The script does not call production arithmetic or scan outside the sanctioned projects root. No API, client, database, or status-board files are changed.
