# Agent-run pricing

Prices are organization-scoped versions keyed by model and `effective_date`. A run uses the latest
version whose date is on or before `started_at`; a future version is never used. If no persisted
version applies, the configured `AGENT_RUN_MODEL_PRICES` value is used as an explicit compatibility
fallback. If neither exists, cost is `null` (unknown), never zero.

`EXCHANGE_RATE_VND_PER_USD` is an optional operator-configured display conversion; it is not an FX
lookup. `cost_usd` is the API-equivalent cost. Billed subscription cost is a separate concept and
must not be inferred from a harness name; callers should retain a zero billed value separately.
