# #505 performance and data-volume review

## Gate commands + output

- `GOCACHE=$(mktemp -d /private/tmp/performance-505-ci.XXXXXX) GOPATH=$(mktemp -d /private/tmp/performance-505-go.XXXXXX) GOMODCACHE=$GOPATH/pkg/mod make ci` — `✓ ci pipeline passed locally` (fmt, tidy, vet, golangci-lint, race tests, build).
- `bash -n scripts/review-performance-505.sh` — pass.

## Smoke/test result

- The executable review harness is `scripts/review-performance-505.sh`. It seeds exactly 50,000 issues, 50,000 pull requests, 10,000 agent runs, and 500,000 timeline steps using `generate_series`, records actual counts and `git rev-parse HEAD`, then measures 20 authenticated requests per API route and reports the nearest-rank p95.
- Measured routes and exact queries are emitted to `.agent-runs/w18r5-performance-505/latency.txt` for overview, work, usage, runs, and board.
- Dashboard first paint is unmeasured: the client/browser Compose fixture was explicitly not reused because another review owns a live fixture.

## Failing control with identical inputs

- Not run. No measured defect was changed in this review, so there is no claimed regression fix or negative control.

## Not covered

- No browser first-paint measurement or client source was used.
- No API p95 values are claimed until the harness is run against a disposable authenticated stack; values above 1 second must become a minimal fix or a linked `found-by:agent` issue.
