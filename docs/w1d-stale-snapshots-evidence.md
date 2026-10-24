# W1d stale snapshot acceptance evidence

## Coverage

The acceptance tests exercise the actual sync and webhook persistence paths for issue, pull request, and pipeline revisions. Sync tests bind a non-zero provider `UpdatedAt` and assert the exact conflict key and freshness update SQL for every entity. Signed webhook ingestion tests assert the same non-zero revision binding for every entity. PostgreSQL acceptance uses production migrations and repository `Upsert` methods for both arrival orders, concurrent writers, missing timestamps, and equal/missing candidates against a known version.

The policy is: a strictly newer non-null upstream revision may update; equal or older revisions do not; a missing revision cannot replace a known revision; two missing revisions remain duplicate-safe. `upstream_updated_at` is separate from local `updated_at`.

## Measured commands and results

Acceptance-test commit before this evidence update: `b5a9d838e06d59218efb0f3fa8dd935bc4b37d9e`.

```sh
cd api
GOCACHE=/tmp/w1d-gocache go test ./cmd/sync ./modules/webhook ./modules/store
```

Result: PASS for all three packages.

Production acceptance baseline, using an owned PostgreSQL 16 Alpine container and `W1D_ACCEPTANCE_DSN`:

```sh
W1D_ACCEPTANCE_DSN='host=127.0.0.1 port=55442 user=w1d password=w1d dbname=w1d sslmode=disable' GOCACHE=/tmp/w1d-gocache go test ./modules/store -run '^TestPostgresStaleSnapshotAcceptance$' -count=1 -v
```

Result: PASS for `newer-first`, `older-first`, missing-timestamp duplicate safety, `concurrent_writers`, and equal/missing protection for issue, pull request, and pipeline. Container: `w1d-accept-20261024-r4`.

Negative control: in a separate disposable worktree, the actual `Where` freshness predicates were removed from the three production repository files and the same acceptance command was run on port `55443`. It failed at intended state/value assertions: `newer-first` expected `closed`, got `open`; concurrent writers expected `closed`, got `open`; equal/missing issue and pull expected `closed`, got `open`; equal/missing pipeline expected `completed`, got `running`. This proves stale state/value corruption rather than fixture setup failure. The control worktree was never committed or pushed.

Cleanup used an EXIT trap with `docker rm -f` for both named containers. After success/failure, `docker ps -a` returned no `w1d-accept-20261024-r4` or `w1d-control-20261024-r4` container. No API process was started and no unrelated service was modified.

## Full API gate

Run sequentially from `api`:

```sh
go vet ./...                 # PASS (exit 0)
golangci-lint run           # PASS (0 issues)
go test ./...               # PASS
go build ./...              # PASS (exit 0)
git diff --check            # PASS (exit 0)
```

## Not covered

Hosted CI and exact-head PR checks are pending the pushed evidence commit. Client, coordinator records, runner, workflow, and unrelated services are not covered by this PR.
