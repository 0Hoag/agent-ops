# W1d stale snapshot evidence

## Implementation

Issues, pull requests, and pipeline runs now persist nullable `upstream_updated_at` separately from local `updated_at`.
Their PostgreSQL `ON CONFLICT` updates use an atomic freshness predicate: a strictly newer non-null upstream timestamp
updates the row; an equal or older timestamp does not; a missing timestamp cannot replace a known timestamp. Two
missing timestamps remain duplicate-safe. Webhook and backfill mappers bind the provider-normalized timestamp.

Migration `003_add_upstream_updated_at.sql` is append-only and adds the three columns and indexes.

## Reproduction

From this worktree:

```text
cd api
GOCACHE=/tmp/w1d-gocache GOMODCACHE=/tmp/w1d-gomodcache go test ./...
GOCACHE=/tmp/w1d-gocache GOMODCACHE=/tmp/w1d-gomodcache go vet ./...
GOCACHE=/tmp/w1d-gocache GOMODCACHE=/tmp/w1d-gomodcache go build ./...
```

Measured result: all tests, vet, and build exit 0. Store SQL mocks assert the `WHERE` freshness predicate for all
three entities; sync and webhook mocks assert the additional upstream timestamp binding; migration tests apply and
record version 003.

Docker 29.2.0 disposable PostgreSQL 16 container `w1d-accept-$(id -u)` was started on 127.0.0.1:55441.
The initial direct SQL exercise demonstrated issue newer-then-older rejection and older-then-newer acceptance;
PR/pipeline SQL fixture attempt failed because duplicate conflict keys were put in one INSERT (PostgreSQL correctly
rejects affecting the same row twice). The real migration runner invocation only ran the migration mock test and did
not migrate the container. This is not a completed production-migration acceptance. Concurrent writers and the
predicate-removal negative control are also not covered.

`golangci-lint run` was attempted, but its typechecking subprocess read the restricted shared Go build cache under
`/Users/zeross/Library/Caches/go-build`; it reported 0 lint issues after the cache access errors and is not claimed
as a passing gate.

## Not covered

- Complete disposable PostgreSQL acceptance against production-applied migrations for all three entities, including
  concurrent writers and both arrival orders.
- Disposable negative control proving stale overwrite after removing the predicate.
- Hosted CI result on the final PR head.
