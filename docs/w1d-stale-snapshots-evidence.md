# W1d stale snapshot evidence

## Implementation

Issues, pull requests, and pipeline runs now persist nullable `upstream_updated_at` separately from local `updated_at`.
Their PostgreSQL `ON CONFLICT` updates use an atomic freshness predicate: a strictly newer non-null upstream timestamp
updates the row; an equal or older timestamp does not; a missing timestamp cannot replace a known timestamp. Two
missing timestamps remain duplicate-safe. Webhook and backfill mappers bind the provider-normalized timestamp.

Migration `003_add_upstream_updated_at.sql` is append-only and adds the three columns and indexes.

## Reproduction

Measured acceptance head: `870c7ca4fd6f342189ef13c4acf6afd16b75d04f`. The focused test uses
`database.RunMigrations` and the actual `store.Issues`, `store.PullRequests`, and `store.PipelineRuns.Upsert` repository
methods against a disposable PostgreSQL 16 container. The test exercises issue/PR/pipeline both arrival orders,
concurrent writers, normalized state/value plus timestamp, equal/missing timestamp policy and null-timestamp
duplicate safety. Webhook and sync mapping tests continue asserting timestamp binding.

```sh
cd api
docker run --detach --rm --name w1d-accept-20261024 -e POSTGRES_USER=w1d -e POSTGRES_PASSWORD=w1d -e POSTGRES_DB=w1d -p 127.0.0.1:55441:5432 postgres:16-alpine
W1D_ACCEPTANCE_DSN='host=127.0.0.1 port=55441 user=w1d password=w1d dbname=w1d sslmode=disable' go test ./modules/store -run '^TestPostgresStaleSnapshotAcceptance$' -count=1 -v
W1D_ACCEPTANCE_DSN='host=127.0.0.1 port=55441 user=w1d password=w1d dbname=w1d sslmode=disable' go test ./modules/store -run '^TestPostgresStaleSnapshotPredicateControl$' -count=1 -v
docker rm -f w1d-accept-20261024
```

Result: PASS for `newer-first`, `older-first`, missing-timestamp duplicate safety for all three entities, `concurrent_writers`, and
`equal_and_missing_timestamps_do_not_overwrite_known_version`. The test applies all embedded production migrations,
including versions 001–003.

Predicate-removal control: `TestPostgresStaleSnapshotPredicateControl` runs a disposable unconditional conflict update
(the production assignment set, with its `WHERE` freshness predicate omitted) and asserts newer issue state/value.
Against the disposable PostgreSQL fixture, it failed as intended: `expected: "closed" actual: "open"`, diagnostic
`predicate removal should expose stale overwrite: expected newer state closed, got "open"`. This distinguishes actual
state corruption from fixture setup failure.

Cleanup: the named container ran with `--rm` and was additionally stopped/removed with `docker rm -f` after both test
runs. Inspected `docker ps` afterward: no `w1d-accept-20261024` container; inspected volume inventory: no volume owned
by this fixture (anonymous volume count unchanged by disposable run). No API process was started. Existing Postgres,
Redis, MongoDB and RabbitMQ containers were only inspected and left untouched.

The full sequential gate `go vet ./... && golangci-lint run && go test ./... && go build ./...` passed at `870c7ca`:
vet exit 0; lint reported `0 issues`; tests passed for sync, store, webhook, database and SCM packages; build exit 0.
Exact-head hosted API CI run `40045549685` completed all five acceptance gates successfully (vet, golangci-lint, test,
build and job completion); `gh run view` showed the job completing while the overall run was still transitioning.

## Not covered

- Exact-head hosted run's terminal conclusion (job gates are successful; overall run was still transitioning at last query).
