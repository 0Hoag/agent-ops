# Worker job logs

`GET /api/v1/workers/{worker_id}/logs` upgrades to an authenticated websocket for an owner or admin of the
job's organization. Messages are JSON objects with `type: "worker.log"` and a `line`, followed by
`type: "worker.completed"` when the launcher exits. A bounded 256-line history is replayed on subscription;
each subscriber has a bounded queue, so a slow client cannot block the child process. Disconnects remove the
subscriber.

The stream is the configured launcher process's stdout and stderr, in arrival order. It does not stream the
detailed Codex/OpenCode harness transcript written by `run-worker.sh` to `.agent-runs/<wave>.jsonl`; that
transcript remains available through the existing agent-run ingestion and reporting paths.
