# Agent-run pricing

Prices are organization-scoped versions keyed by model and `effective_date`. A run uses the latest
version whose date is on or before `started_at`; a future version is never used. If no persisted
version applies, the configured `AGENT_RUN_MODEL_PRICES` value is used as an explicit compatibility
fallback. If neither exists, cost is `null` (unknown), never zero.

`EXCHANGE_RATE_VND_PER_USD` is an optional operator-configured display conversion; it is not an FX
lookup. `cost_usd` is the API-equivalent cost. Billed subscription cost is a separate concept and
must not be inferred from a harness name; callers should retain a zero billed value separately.

## Run explorer

`GET /api/v1/agent-runs` requires an organization session and returns `data` plus
`meta.page`, `meta.per_page`, and `meta.total`. Filters are conjunctive: `repository_id`, `wave`,
`harness`, `model`, positive `issue_number`, positive `pr_number`, and RFC3339
`started_after`/`started_before`. Results are ordered by `started_at DESC, id DESC`. Each row may
include repository-matched `relations` for issues, pull requests, and CI runs; numbers are never
resolved across repositories.

`GET /api/v1/agent-runs/{run_id}/timeline` is organization-scoped. Machine ingestion uses
`POST /api/v1/agent-runs/{run_id}/timeline` with `X-Agent-Run-Token`; the payload contains only
ordered usage counters, timestamps when sourced, cache-hit state, and tool names. Prompt text,
tool arguments/results, secrets, and raw transcript content are not accepted or persisted.
