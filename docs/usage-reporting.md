# Worker usage reporting

Both worker launchers always retain their JSONL and summary locally. Reporting is enabled only when
`AGENT_RUN_API_URL`, `AGENT_RUN_TOKEN`, and `AGENT_RUN_REPOSITORY_ID` are all configured. Optional
`AGENT_RUN_ISSUES` is a JSON array of issue numbers; `AGENT_RUN_MODEL` overrides the harness default.
The token is sent only as `X-Agent-Run-Token` and is never printed. API failures are best-effort and
do not replace the worker's exit status.

Codex `input_tokens` includes cached input; `cached_input_tokens` is its subset, not added again.
OpenCode input is likewise the reported total, with cache.read as a subset; reasoning is recorded
separately and is not added to output.
