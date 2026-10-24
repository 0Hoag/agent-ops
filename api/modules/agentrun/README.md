# Agent runs API

`POST /api/v1/agent-runs` accepts normalized token usage and requires `X-Agent-Run-Token`, configured by `AGENT_RUN_TOKEN`. All token fields are non-negative; `ended_at` cannot precede `started_at`.

`GET /api/v1/agent-runs` lists runs, optionally filtered by `repository_id`, `wave`, and `issue_number`. `GET /api/v1/agent-runs/aggregate` returns totals grouped by wave, worker, and each issue number (or a null issue for runs without issues). Both endpoints accept repository and wave filters.

`AGENT_RUN_MODEL_PRICES` is JSON keyed by model. `input_per_million`, `cached_input_per_million`, `cache_write_per_million`, `output_per_million`, and `reasoning_per_million` are USD per one million tokens. Input counters are mutually exclusive normalized categories: cached input is not included in input tokens, and cache-write usage is distinct. Unconfigured model cost is `null` (unknown), not zero. Aggregate cost is unknown if any contributing run for that group has an unknown model price.

## HTTP acceptance

From the repository root, run `scripts/agent-run-acceptance.sh`. It builds the API into a temporary
binary, starts that owned process directly, and starts a uniquely named PostgreSQL Compose project on
an available host port. It asserts authenticated HTTP writes, normalized SQL fields, exact filtered run
identities and the complete aggregate snapshot. Cleanup verifies API PID/listener and owned container/
volume absence on every exit. Set `AGENT_RUN_ACCEPTANCE_API_PORT` and
`AGENT_RUN_ACCEPTANCE_PG_PORT` to choose free ports; PostgreSQL host port 5432 is refused.

The disposable mutation controls prove the acceptance can detect defects without changing production:

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
```

The exact-token case-only control is already covered by `TestCreateValidationPrecedesRepositoryAccess`
from #192 and is intentionally not duplicated here.
