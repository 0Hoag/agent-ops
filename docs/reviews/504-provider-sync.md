# #504 provider sync review

## Gate commands + output

- `GOCACHE=$(mktemp -d /private/tmp/provider-504-ci.XXXXXX) GOPATH=$(mktemp -d /private/tmp/provider-504-mod.XXXXXX) GOMODCACHE=$GOPATH/pkg/mod make ci` — `✓ ci pipeline passed locally` (fmt, tidy, vet, golangci-lint, race tests, build).
- The first unmodified-cache attempt failed before compilation because the sandbox denied writes to `~/Library/Caches/go-build` and the module cache; the passing run used disposable writable caches.

## Smoke/test result

- Targeted: `GOCACHE=$(mktemp -d /private/tmp/provider-504-testcache.XXXXXX) go test -count=1 ./modules/repositorysync -run TestSyncPersistsPipelineRunsWithProviderIdentity -v` — `PASS`.
- Existing provider HTTP contract tests pass for GitHub and GitLab pagination, request paths/query strings, state normalization, labels, assignees, and pipeline mapping: `go test ./modules/repositorysync ./pkg/handlers/scm/...` — `PASS`.
- The production fix persists every normalized provider pipeline run through `PipelineRunRepository.Upsert`, preserving provider external ID, PR number, SHA, status, conclusion, and timestamps. Existing PostgreSQL upsert conflict/update and repository cascade-delete tests remain in the full gate.

## Failing control with identical inputs

- Not run in this implementation pass. The targeted regression is unit-level SQL expectation coverage; a disposable real-PostgreSQL control mutating only the production pipeline persistence guard remains required before acceptance.

## Not covered

- The requested >100-entity GitHub and GitLab HTTP-stub matrix with second changed-data `SyncAll`/manual sync, 429/5xx, invalid-token, exact persisted counts/fields, and real PostgreSQL was not run here.
- Repository-settings HTTP regression and scheduled-sync error-path coverage were not expanded here.
- No browser, Docker Compose, or client gate was in scope; no such command was run.
