# Worker usage reporting

Both worker launchers always retain their JSONL, normalized usage summary (`.agent-runs/<wave>-usage.json`),
and final-report file locally. Reporting is enabled only when
`AGENT_RUN_API_URL`, `AGENT_RUN_TOKEN`, and `AGENT_RUN_REPOSITORY_ID` are all configured. Optional
`AGENT_RUN_ISSUES` is a JSON array of issue numbers; `AGENT_RUN_MODEL` overrides the harness default.
The token is sent only as `X-Agent-Run-Token` and is never printed. API failures are best-effort and
do not replace the worker's exit status.

Codex `input_tokens` is copied from the completed-turn `input_tokens` field, and
`cached_input_tokens` is copied separately from `cached_input_tokens`; neither is recomputed or added.
OpenCode `input_tokens` is copied from `step_finish.part.tokens.input`, `cached_input_tokens` from
`part.tokens.cache.read`, and cache writes from `part.tokens.cache.write`. These are passed as reported
fields without assuming cache reads are a subset or subtracting them from input. `reasoning` is a
separate reported token count and is not added to output. Absent usage fields normalize to zero.

Run `bash scripts/usage-reporting-fixtures.sh` for synthetic executable-shim tests through both real
launchers and a local fake HTTP endpoint; it does not start an agent.
