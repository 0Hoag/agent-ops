# Signed webhook load test

Run `bash scripts/test-webhook-load.sh`. It provisions an isolated, uniquely named Postgres/API fixture, sends 50 fresh signed GitHub `issues` deliveries per second for 30 seconds, reconciles the exact numeric SQL `webhook_events` count with successful HTTP responses, writes `.agent-runs/w16i-webhook-load/report.txt`, and removes/verifies owned containers and volumes on every exit.

Prerequisites are Docker Compose, `curl`, `lsof`, Python 3, and access to `grafana/k6`. Override `W16I_RATE`, `W16I_DURATION`, `W16I_API_PORT`, `W16I_POSTGRES_PORT`, or `W16I_ARTIFACT_DIR` for a measured experiment. Secrets come from environment variables and are not logged.

The run fails on dropped iterations, non-2xx responses (including the existing repository rate limiter's 429), signature rejection, or a mismatch between successful responses and the persisted SQL count. `report.txt` records measured p95 and throughput; the default target is not a performance promise.
