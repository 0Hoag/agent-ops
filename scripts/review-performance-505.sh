#!/usr/bin/env bash
set -euo pipefail

# Isolated API data-volume review. Run from the backend repository with a disposable
# PostgreSQL database and an authenticated API session:
#   DATABASE_URL=postgres://... API_URL=http://127.0.0.1:18080 \
#   SESSION_TOKEN=... REPOSITORY_ID=... scripts/review-performance-505.sh
# Docker/Compose callers must invoke this script through scripts/heavy.sh.

: "${DATABASE_URL:?set DATABASE_URL for the disposable review database}"
: "${API_URL:?set API_URL for the review API}"
: "${SESSION_TOKEN:?set SESSION_TOKEN for an organization session}"
: "${REPOSITORY_ID:?set the seeded repository UUID}"
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ARTIFACT_DIR=${ARTIFACT_DIR:-"$ROOT/.agent-runs/w18r5-performance-505"}
mkdir -p "$ARTIFACT_DIR"
export PGDATABASE=${PGDATABASE:-agent_ops}

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<SQL
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM repositories WHERE id = '$REPOSITORY_ID') THEN
    RAISE EXCEPTION 'REPOSITORY_ID does not exist: %', '$REPOSITORY_ID';
  END IF;
END
$$;
INSERT INTO issues (id, repository_id, number, title, state, status, labels, created_at, updated_at)
SELECT gen_random_uuid(), '$REPOSITORY_ID', n, 'performance issue ' || n, 'open', 'todo', '["status:todo"]'::jsonb,
       '2026-11-01'::timestamptz + (n % 30) * interval '1 hour', '2026-11-01'::timestamptz + (n % 30) * interval '1 hour'
FROM generate_series(1, 50000) n ON CONFLICT (repository_id, number) DO NOTHING;
INSERT INTO pull_requests (id, repository_id, number, title, state, head_sha, branch, closes, created_at, updated_at)
SELECT gen_random_uuid(), '$REPOSITORY_ID', n, 'performance pull request ' || n, 'open', md5(n::text), 'perf/' || n, '[]'::jsonb,
       '2026-11-01'::timestamptz + (n % 30) * interval '1 hour', '2026-11-01'::timestamptz + (n % 30) * interval '1 hour'
FROM generate_series(1, 50000) n ON CONFLICT (repository_id, number) DO NOTHING;
INSERT INTO agent_runs (id, repository_id, wave, worker, model, issue_numbers, pr_number, input_tokens, cached_input_tokens, cache_write_tokens, output_tokens, reasoning_tokens, started_at, ended_at)
SELECT gen_random_uuid(), '$REPOSITORY_ID', 'W18R5', 'review-worker', 'review-model', jsonb_build_array((n % 50000) + 1), (n % 50000) + 1,
       100, 20, 5, 40, 10, '2026-11-01'::timestamptz + (n % 30) * interval '1 hour', '2026-11-01'::timestamptz + (n % 30) * interval '1 hour' + interval '5 minutes'
FROM generate_series(1, 10000) n ON CONFLICT DO NOTHING;
INSERT INTO agent_run_steps (id, run_id, sequence, started_at, ended_at, input_tokens, cached_input_tokens, output_tokens, tool_name)
SELECT gen_random_uuid(), r.id, s.n, r.started_at + s.n * interval '1 second', r.started_at + s.n * interval '1 second' + interval '1 second', 10, 2, 4, 'review'
FROM agent_runs r CROSS JOIN generate_series(1, 50) s(n)
WHERE r.repository_id = '$REPOSITORY_ID' AND r.wave = 'W18R5'
  AND NOT EXISTS (SELECT 1 FROM agent_run_steps x WHERE x.run_id = r.id AND x.sequence = s.n);
ANALYZE issues; ANALYZE pull_requests; ANALYZE agent_runs; ANALYZE agent_run_steps;
SQL

counts=$(psql "$DATABASE_URL" -At -F '|' -c "SELECT (SELECT count(*) FROM issues WHERE repository_id='$REPOSITORY_ID'), (SELECT count(*) FROM pull_requests WHERE repository_id='$REPOSITORY_ID'), (SELECT count(*) FROM agent_runs WHERE repository_id='$REPOSITORY_ID' AND wave='W18R5'), (SELECT count(*) FROM agent_run_steps s JOIN agent_runs r ON r.id=s.run_id WHERE r.repository_id='$REPOSITORY_ID' AND r.wave='W18R5')")
printf 'revision=%s\ncounts issues|prs|runs|steps=%s\n' "$(git -C "$ROOT" rev-parse HEAD)" "$counts" | tee "$ARTIFACT_DIR/seed.txt"

measure() {
  local name=$1 path=$2
  : >"$ARTIFACT_DIR/$name.samples"
  for _ in $(seq 1 20); do
    curl --fail --silent --show-error -o /dev/null -w '%{time_total}\n' -H "Authorization: Bearer $SESSION_TOKEN" "$API_URL$path" >>"$ARTIFACT_DIR/$name.samples"
  done
  p95=$(sort -n "$ARTIFACT_DIR/$name.samples" | awk 'NR==int(0.95*20+0.999) {print $1}')
  printf '%s p95_seconds=%s samples=20 query=%s\n' "$name" "$p95" "$path" | tee -a "$ARTIFACT_DIR/latency.txt"
}
measure overview "/api/v1/overview?repository_id=$REPOSITORY_ID&from=2026-11-01&to=2026-11-30"
measure work "/api/v1/work?repository_id=$REPOSITORY_ID&page=1&per_page=50"
measure usage "/api/v1/insights/analytics?repository_id=$REPOSITORY_ID&from=2026-11-01&to=2026-11-30&period=daily&group_by=model"
measure runs "/api/v1/agent-runs?repository_id=$REPOSITORY_ID&page=1&per_page=50"
measure board "/api/v1/board?repository_id=$REPOSITORY_ID&page=1&per_page=50"
printf 'dashboard_first_paint=UNMEASURED (client/browser fixture deliberately not reused)\n' | tee -a "$ARTIFACT_DIR/latency.txt"
