# Agent runs API

`POST /api/v1/agent-runs` accepts normalized token usage and requires `X-Agent-Run-Token`, configured by `AGENT_RUN_TOKEN`. The API binds this machine-authenticated write to the persisted repository whose `full_name` is present in `SCM_REPOSITORIES_JSON`; the request body cannot select an arbitrary tenant repository. All token fields are non-negative; `ended_at` cannot precede `started_at`.

`harness` and `session_id` are optional identity fields for new producers. When both are present,
the database enforces one row per `(repository_id, harness, session_id)` and retries atomically update
the cumulative counters and metadata. Legacy producers that omit either field retain insert behavior.

The `agentops push` CLI reads a Codex, OpenCode, or Claude JSONL transcript and submits normalized
sessions. Example: `AGENT_RUN_TOKEN=... AGENT_RUN_REPOSITORY_ID=<uuid> agentops push --harness codex
--wave W9d --worker agentops-push transcript.jsonl`. `AGENT_RUN_API_URL` overrides the default endpoint;
`--endpoint`, `--repository-id`, `--wave`, and `--worker` are available as explicit flags. The CLI never
prints the token or transcript contents and exits nonzero for invalid input or failed HTTP submission.

`GET /api/v1/agent-runs` lists runs, optionally filtered by `repository_id`, `wave`, and `issue_number`. `GET /api/v1/agent-runs/aggregate` returns totals grouped by wave, worker, and each issue number (or a null issue for runs without issues). Both endpoints accept repository and wave filters.

`AGENT_RUN_MODEL_PRICES` is JSON keyed by model. `input_per_million`, `cached_input_per_million`, `cache_write_per_million`, `output_per_million`, and `reasoning_per_million` are USD per one million tokens. Input counters are mutually exclusive normalized categories: cached input is not included in input tokens, and cache-write usage is distinct. Unconfigured model cost is `null` (unknown), not zero. Aggregate cost is unknown if any contributing run for that group has an unknown model price.

## HTTP acceptance

From the repository root, run `scripts/agent-run-acceptance.sh`. It builds the API into a temporary
binary, starts that owned process directly, and starts a uniquely named PostgreSQL Compose project on
an available host port. It asserts authenticated HTTP writes, normalized SQL fields, exact filtered run
identities and the complete aggregate snapshot. Cleanup verifies API PID/listener and owned container/
volume absence on every exit; UTC `started_at` and `ended_at` are compared against the persisted SQL
row. Set `AGENT_RUN_ACCEPTANCE_API_PORT` and
`AGENT_RUN_ACCEPTANCE_PG_PORT` to choose free ports; PostgreSQL host port 5432 is refused.

The disposable mutation controls prove the acceptance can detect defects without changing production.
Ordinary controls compare the intended target against its baseline before running nested acceptance, so
an unchanged mutation is rejected immediately. They also require distinct API PID, listener, and owned
container/volume cleanup evidence and reject any cleanup failure:

```bash
scripts/agent-run-acceptance-control.sh auth
scripts/agent-run-acceptance-control.sh validation
scripts/agent-run-acceptance-control.sh repository-list
scripts/agent-run-acceptance-control.sh repository-aggregate
scripts/agent-run-acceptance-control.sh wave-list
scripts/agent-run-acceptance-control.sh wave-aggregate
scripts/agent-run-acceptance-control.sh issue-list
scripts/agent-run-acceptance-control.sh issue-aggregate
scripts/agent-run-acceptance-control.sh pricing
scripts/agent-run-acceptance-control.sh noop
scripts/agent-run-acceptance-control.sh timestamp
scripts/agent-run-acceptance-control.sh cleanup-failure
scripts/agent-run-acceptance-control.sh cleanup-remove
scripts/agent-run-acceptance-control.sh cleanup-omit-pid
scripts/agent-run-acceptance-control.sh cleanup-omit-listener
scripts/agent-run-acceptance-control.sh cleanup-omit-fixtures
scripts/agent-run-acceptance-control.sh cleanup-missing-injection
scripts/agent-run-acceptance-control.sh cleanup-extra-failure
```

The `noop` control accepts only an unchanged target rejected before nested acceptance. Run
`scripts/agent-run-acceptance-control-regression.sh` for standalone behavioral wrapper regressions.
The timestamp control corrupts the persisted row and must fail its exact SQL assertion. The cleanup-failure
control combines a failing list assertion with an injected cleanup failure; the wrapper must observe both
diagnostics while verifying real fixture cleanup. The omission controls remove each PID, listener, and
container/volume success diagnostic in turn. The missing-injection control causes a real timestamp assertion
failure while disabling the requested synthetic cleanup injection. The cleanup-extra-failure control combines
the intended list assertion and expected synthetic cleanup failure with an additional cleanup failure; each
invalid-evidence control must make the actual wrapper exit nonzero. `cleanup-remove` skips Compose removal
only in a disposable run, proves the actual fixture is detected as retained, then runs outer Compose removal
and verifies container and volume absence. It also checks process and listener evidence from that same cleanup.

The exact-token case-only control is already covered by `TestCreateValidationPrecedesRepositoryAccess`
from #192 and is intentionally not duplicated here.
