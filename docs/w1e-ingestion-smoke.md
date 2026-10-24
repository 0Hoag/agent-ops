# W1e signed ingestion smoke

From a clean checkout, run:

```sh
bash scripts/ingestion-smoke.sh
```

Requirements: Docker, Go, `curl`, Python 3, and OpenSSL. The script builds `api/` from this checkout, starts a uniquely named PostgreSQL 17 fixture published on a dynamically assigned loopback-only port, and binds the API to a dynamically selected loopback port. It configures repositories `w1e/a` and `w1e/b` with distinct local-only secrets, waits for PostgreSQL and API readiness, submits signed GitHub issue webhooks, then verifies persisted rows directly in PostgreSQL. It never reads externally sourced payloads or prints fixture secrets or request bodies.

The assertions cover A signed with A (`200`, `accepted:true`, exactly one raw delivery and issue with expected repository, number, state, and normalized status `done`), A signed with B (`400`), signed unknown repository (`400`), and replay of the original delivery (`200`, `accepted:false`). The A/A check compares the complete sorted SQL snapshot, including exact numeric counts; rejections and replay must leave that snapshot unchanged.

Cleanup is installed before fixture creation and runs on normal exit, assertion failure, and handled signals. It stops and waits for the API process, removes only its recorded container and anonymous volume, deletes its temporary files, verifies those owned resources are absent, and compares Docker container/volume inventories against the initial snapshot to detect leftovers. The command fails rather than pruning or stopping unrelated services. For a deliberate failure-control run (expected nonzero exit with `ASSERTION FAILED` and the same cleanup evidence):

```sh
W1E_FORCE_ASSERTION_FAILURE=1 bash scripts/ingestion-smoke.sh
```

The failure hook is test-only and does not alter source or API behavior. To prove the exact persisted-snapshot assertion detects extra rows, run:

```sh
W1E_EXTRA_ROW_CONTROL=1 bash scripts/ingestion-smoke.sh
```

This control inserts one extra raw delivery and issue after A/A; it must fail with the expected-vs-actual snapshot diagnostic showing `raw|2` and `issue|2`, then verify the same owned-resource cleanup.
