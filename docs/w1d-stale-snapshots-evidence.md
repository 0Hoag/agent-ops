# W1d stale snapshot acceptance evidence

## Coverage

Sync backfill tests bind non-zero provider `UpdatedAt` values and assert the conflict keys and every original update assignment plus `upstream_updated_at`/freshness predicate for issue, pull request, and pipeline. Their disposable controls remove one sync mapper revision at a time; the focused test fails on the intended argument mismatch (`expected ... 2026-01-02 03:00:00 UTC`, `actual <nil>`).

Webhook coverage calls `persistEvent` directly with a non-zero revision, asserting the bound timestamp for all three entities. This is service-persistence coverage, not a signed-POST test. Existing HMAC and repository-binding tests remain unchanged. In a disposable worktree, removing only the issue, pull-request, or pipeline `UpstreamUpdatedAt: upstreamTime(...)` mapping in turn made its corresponding focused subtest fail on the timestamp argument (`expected [time.Time ...]`, `actual [<nil>]`).

PostgreSQL acceptance runs actual production migrations and the actual issue, pull-request, and pipeline `Upsert` repositories. It covers both arrival orders, concurrent writers, state/value/revision, equal and missing candidates for each entity, and duplicate missing timestamps. A separate disposable worktree removed all three production repository `Where` freshness predicates (no handwritten SQL substitution); the same acceptance test failed with stale values: `newer-first` issue `closed`→`open`, concurrent issue `closed`→`open`, equal/missing issue and pull request `closed`→`open`, pipeline `completed`→`running`.

## Measured commands and results

API test commit: `a01672de236a4f6cb7d078517927cec3b4de47f1`.

```sh
cd api
GOCACHE=/tmp/w1d-gocache go test ./cmd/sync ./modules/webhook ./modules/store
# PASS: all three packages
go vet ./...
# PASS (exit 0)
golangci-lint run
# PASS (0 issues)
go test ./...
# PASS: all packages
go build ./...
# PASS (exit 0)
git diff --check
# PASS (exit 0)
```

Production PostgreSQL 16 baseline:

```sh
W1D_ACCEPTANCE_DSN='host=127.0.0.1 port=55443 user=w1d password=w1d dbname=w1d sslmode=disable' \
  go test ./modules/store -run '^TestPostgresStaleSnapshotAcceptance$' -count=1 -v
```

Result: PASS for `newer-first`, `older-first`, duplicate missing timestamps, `concurrent writers`, and equal/missing policy for issue, pull request, and pipeline. The test applies all production migrations (001–003). Disposable actual-predicate-removal control: the same test failed on the stale state/value diagnostics listed above, not on migration or fixture setup.

Both final acceptance runs installed EXIT cleanup using `docker rm -fv`: baseline fixture `w1d153-exit-base` passed (exit 0), and actual-predicate-removal fixture `w1d153-exit-control` failed as intended (exit 1). The baseline anonymous volume `0de1c1a900fc396556abec5e58e053ed1d8e466cba744ea75dcfdd49466e65b0` and control anonymous volume `82e2bd6690c0e3d17928b9c49e53d7371bf3bcee8afafd9493934a9c6b0b6b35` were each confirmed absent by `docker volume inspect`; `docker ps -a` showed neither owned container. Earlier trial fixtures `w1d153-baseline`, `w1d153-baseline2`, `w1d153-control`, and `w1d153-exit-control` were also verified absent with their anonymous volumes; no unrelated containers were changed.

Hosted API CI on exact test head `a01672de236a4f6cb7d078517927cec3b4de47f1`: run `40048072350`, `API` job PASS.

## Not covered

Client, coordinator records, runner, workflow, and unrelated services are outside scope. Hosted CI for the final evidence-document commit must pass on its exact head.
