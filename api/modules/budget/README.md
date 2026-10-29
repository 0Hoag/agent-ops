# Budgets

Budgets are organization-scoped persisted limits for a whole organization, repository, or wave. A repository selector must belong to the authenticated organization. `token_limit` and `usd_limit` are independent; at least one is required. `starts_at` is inclusive and `ends_at` is exclusive for future measurement consumers. `Evaluate` fires each rule at `usage >= 80%` and `usage >= 100%`; it is deterministic and does not read clocks or external services.
