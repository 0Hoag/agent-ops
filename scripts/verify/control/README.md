# Control lifecycle verifier (#518)

Run `bash scripts/verify/control.sh` from any directory. Requires Docker, Python 3,
curl, and the repository's canonical `scripts/heavy.sh`. Every Compose invocation
is serialized internally; do not wrap the verifier in heavy.sh.

The verifier builds an isolated API image once and reuses it. Its unique Compose
project owns a private migrated Postgres volume, an unpublished database, and a
loopback API port selected by the OS. Three disposable accounts exercise owner,
member, and foreign-tenant access. No existing database or account is used.

`WORKER_SCRIPTS_JSON` replaces the complete launcher map with `/fixture/stub.sh`.
`WORKER_SCRIPT_ROOTS` and `WORKER_WORKTREE_ROOTS` restrict requests to `/fixture`.
These variables are consumed by `pkg/config.Load`, wired by `main.go` into the
worker executor, and checked before `exec.CommandContext` executes the configured
script with wave, branch, and absolute brief arguments. The stub validates those
arguments, emits known stdout/stderr lines, and either exits or replaces itself
with sleep. It never launches a real agent. Stopped jobs have no descendant
processes; all fixture processes are confined to the owned API container.

Checks cover wave creation, valid/invalid transitions, listing and deletion;
worker queued/running/cancelled/succeeded states and distinct restart IDs; tenant
and role denial before WebSocket upgrade; real RFC6455 handshake and exact live
and replay line order/completeness; audit action/job/actor/tenant/timestamp,
pagination, SQL counts, and identical history after API restart.

The production mutation is confined to a temporary API source copy. It permits
`todo -> done`, replaces only the API image on the same stack, and sends the same
PATCH to the same stored wave. The normal HTTP status oracle must fail at
`WAVE_INVALID_TRANSITION_REJECTED` (400 expected, 200 actual). The complete row is
restored from a database snapshot, the original image is reused, and the same
probe must pass. The verifier itself succeeds only after this failing control and
restored pass. Production source files are never modified.

Evidence defaults to `.agent-runs/W19j-control-518/<run>/`; override with
`CONTROL_VERIFY_ARTIFACT_DIR`. Logs contain the tested SHA, exact checks, audit
rows, WebSocket messages, mutation failure, restored pass, and cleanup results.
Archive this directory under the canonical repository's `.agent-runs` before
removing the worktree. If sandbox permissions prevent archival, retain the
worktree and report the evidence location. Cleanup runs on failure and success,
removes the owned stack and volume, and checks zero owned containers, volumes,
and networks. Build images remain available as Docker build cache.

Not covered: real Codex/OpenCode execution or harness transcript ingestion;
queue saturation; slow-subscriber overflow beyond the documented bounded queue;
more than 256 replay lines; production hosting and browser UI. The wave state is
explicitly managed through its HTTP API; it is not inferred from worker state.
