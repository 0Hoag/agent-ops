# Budget and alert runtime verification (#517)

Run `bash scripts/verify/alerts.sh` from an isolated worktree. It builds the production API once,
starts a unique Compose project with migrated PostgreSQL, bootstraps a real account, selects its
organization over HTTP, and creates a budget, thresholds and Slack channel over HTTP. Only the
external Slack HTTP receiver is stubbed. SQL fixtures represent persisted run observations;
`GET /agent-runs` independently verifies their public representation. No internal Go function is invoked.

The production runtime has a one-minute poll and 100-repository page size, hard-coded in
`api/main.go` and `modules/alert/runtime.go`. There is no invented interval environment setting.
The 101-repository fixture exercises scan progress. API restarts reuse the same database and image.
All Compose commands acquire `/Users/zeross/agent-ops/scripts/heavy.sh` individually; do not wrap
this verifier in that helper. Expected runtime is several minutes plus the initial image build.

Set `ALERTS_VERIFY_ARTIFACT_DIR` to a new absolute directory to retain HTTP responses, channel
requests, expected/actual comparisons, API logs, source SHA and cleanup inventory. Redirect stdout
and stderr to a sibling command log if desired. Keep each invocation's evidence separate.
The final checks also cross 100% and observe recovery followed by a fresh failure.

Copy evidence to `/Users/zeross/agent-ops/.agent-runs/W19i-alerts-517/` before worktree removal.
The exit trap removes owned Compose containers, volumes, networks and locally built images and terminates the stub.
It explicitly enumerates resources after teardown, including when assertions fail.

Set `ALERTS_VERIFY_CONTROL=1` for the negative control. A disposable API copy changes only the
production low-cache predicate to always false. The same fixture and normal `low-cache occurrence
first scan` oracle must report expected 100, actual 0. The source checkout is never mutated.
Run the default command again to restore the production behavior. Existing production contract
gaps still fail; a restored run is not called green unless all assertions pass.

Not covered: a browser, real external messaging services, Telegram/SMTP, USD conversion, pricing,
Work attribution, active-worker ingestion, and every operational alert rule. The SQL fixture is
not a claim of machine-ingestion acceptance. Budget alerts and retries are strict assertions even
when the current product does not implement them. No alternate internal-function oracle is used.
