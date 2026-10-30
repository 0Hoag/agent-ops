# Webhook module

`webhook` receives signed SCM webhook requests, verifies and parses them through the provider interface, then persists accepted deliveries and normalized entities through `store`.

## Composition and injection

The application composes `NewService(db, registry)`, `NewController(service, configs)`, and `NewRoutes(controller)`, then calls `Register` on the API route group. `ProviderRegistry` is injected at the service boundary; repository configuration (repository name and secret/token/base-URL environment-variable names) is injected into the controller. The controller reads credentials from the environment at request time; configuration carries references, not secret values.

The service depends on GORM for the encompassing persistence transaction and uses `store` for entity/event repositories. It does not own database connection setup, migrations, provider registration, or HTTP server lifecycle. The caller supplies request contexts and the service propagates them to provider and database operations.

## Rate limiting

After signature verification, repository binding, and event validation, accepted deliveries use a process-local token bucket keyed by the persisted repository ID. The default bucket allows 50 deliveries per second per repository, with a 50-delivery burst, and returns HTTP 429 when empty. Tokens refill continuously; unrelated repositories have independent buckets. Storage is bounded to 4096 repository IDs with least-recently-seen eviction, and no background cleanup goroutine is used. Invalid signatures and payloads are rejected before a bucket is charged.

## Persistence and ingestion boundary

The service verifies provider signatures before parsing, requires the parsed repository to match a configured repository, and persists a delivery and its normalized issue/pull-request/pipeline record atomically. Delivery identity enforces idempotency; duplicate deliveries are accepted without re-applying entity writes. Schema and indexes are owned by numbered migrations in `api/migrations`; normalized storage models live in `modules/store`. Preserve this ordering and transaction behavior when changing ingestion.

## Source-parity audit

Reference: read-only `/Users/zeross/Desktop/controlplane/modules/team/` and `docs/code-standards.md`, which uses a controller/repository/routes/README layout for an HTTP business module. Webhook is an ingestion adapter rather than a CRUD resource module, so only applicable layers are expected.

| Concern / expected layer | Webhook evidence | Assessment |
| --- | --- | --- |
| HTTP controller | `controller.go` | Present: bounded request read, configured provider selection, verification/ingestion, response mapping. |
| Routes | `routes.go` | Present: registers the provider webhook endpoint on caller-supplied route group. |
| Service | `service.go` | Present: provider verification/parsing, repository matching, validation, atomic persistence orchestration. |
| Service tests | `service_test.go`, `service_revision_test.go` | Present for ingestion/provider behavior; no tests added solely for documentation. |
| Models / DTO types | — | Not applicable: normalized persistence models/types belong to `modules/store`; webhook config and provider events are represented at their respective boundaries. |
| Repository files | — | Not applicable: persistence operations are deliberately delegated to `modules/store`, avoiding a second database abstraction. |
| Standalone webhook migrations | — | Not applicable: schema lifecycle is centralized in `api/migrations`, not module-local. |
| README | `README.md` | Added: dependency injection, credentials, transaction and persistence boundaries documented. |
