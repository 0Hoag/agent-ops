# Store module

`store` is the persistence boundary for repositories, issues, pull requests, pipeline runs, and webhook events. It contains database models and GORM repositories only; provider parsing, HTTP handling, and application wiring belong to their respective modules/caller.

## Composition and injection

Construct the module with `store.New(db)` and inject the resulting `*Store` into the consumer. The constructor exposes typed repositories (`Repositories`, `Issues`, `PullRequests`, `PipelineRuns`, and `WebhookEvents`) while retaining the shared `*gorm.DB` internally. Use `Store.WithTx(tx)` when a caller has an existing transaction; all returned repositories then operate on that transaction. The store does not open connections, run migrations, or own transaction lifecycle.

Repository methods accept `context.Context` and apply it through GORM. Callers own transaction boundaries and decide which operations must be atomic. `JSONB` validates values on writes and supports database byte/string reads; `UUID` is a string representation with PostgreSQL enforcing persisted UUID validity.

## Persistence boundary

Models map to the existing schema in `api/migrations`; schema changes belong in numbered migrations, not model-driven auto-migration. Repository methods bind values as SQL parameters and implement ingestion upserts/idempotency for their entity keys. Provider-specific data is translated into these provider-neutral models before persistence. `Repository.WebhookSecretEnvRef` stores an environment-variable reference, never the secret itself.

## Source-parity audit

Reference: read-only `/Users/zeross/Desktop/controlplane/modules/team/` and `docs/code-standards.md` (file layout, repository patterns, module README). `team` is a feature-rich CRUD/permission module; this module is a persistence package, so handler/service concerns are intentionally not copied.

| Concern / expected layer | Store evidence | Assessment |
| --- | --- | --- |
| Models | `models.go` | Present for all five persisted entities; schema remains migration-owned. |
| Shared types | `types.go` | Present for UUID, JSONB, and paging. |
| Repository composition | `store.go` | Present: `New`, typed repositories, and transaction rebinding with `WithTx`. |
| Entity repositories | `repository_repositories.go`, `repository_issues.go`, `repository_pull_requests.go`, `repository_pipeline_runs.go`, `repository_webhook_events.go` | Present, split by entity; context passed to DB operations and errors wrapped. |
| Repository tests | `repository_*_test.go`, `postgres_acceptance_test.go` and helpers | Present for entity behavior and PostgreSQL acceptance; no extra tests added for documentation. |
| Controller / routes | — | Not applicable: store has no HTTP/API responsibility; webhook owns request handling and routing. |
| Service | — | Not applicable: persistence methods do not orchestrate provider or business workflows. |
| README | `README.md` | Added: composition, injection, transaction and persistence ownership documented. |
