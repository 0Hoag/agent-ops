#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT="agent-ops-w16i-webhook-load-$(id -u)-$$"; PG_PORT=${W16I_POSTGRES_PORT:-$((25000+RANDOM%1000))}; API_PORT=${W16I_API_PORT:-$((26000+RANDOM%1000))}
ARTIFACT_DIR=${W16I_ARTIFACT_DIR:-"$ROOT/.agent-runs/w16i-webhook-load"}; mkdir -p "$ARTIFACT_DIR"; SECRET=${W16I_WEBHOOK_SECRET:-w16i-webhook-secret}; REPOSITORY="w16i/load-$PROJECT"
compose(){ env POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$PG_PORT" SMOKE_API_PORT="$API_PORT" JWT_SECRET=w16i-disposable-jwt REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 SCM_REPOSITORIES_JSON="[{\"provider\":\"github\",\"full_name\":\"$REPOSITORY\",\"organization_id\":\"d0000000-0000-4000-8000-000000000011\",\"webhook_secret_env\":\"W16I_WEBHOOK_SECRET\"}]" W16I_WEBHOOK_SECRET="$SECRET" docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" "$@"; }
cleanup(){ local rc=$?; trap - EXIT INT TERM; compose down --volumes --remove-orphans >"$ARTIFACT_DIR/cleanup.log" 2>&1 || rc=1; [[ -z "$(docker ps -a --filter "label=com.docker.compose.project=$PROJECT" --format '{{.ID}}')" ]] || rc=1; [[ -z "$(docker volume ls --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Name}}')" ]] || rc=1; [[ $rc -eq 0 ]] && echo "cleanup verified: $PROJECT" || echo "cleanup failed: $PROJECT" >&2; exit "$rc"; }
trap cleanup EXIT INT TERM; export W16I_WEBHOOK_SECRET="$SECRET"; compose up -d postgres
for _ in $(seq 1 60); do compose exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
compose up -d --build api; for _ in $(seq 1 60); do curl -fsS "http://127.0.0.1:$API_PORT/readyz" >/dev/null && break; sleep 1; done
echo 'control: rejecting endpoint must fail k6'; if docker run --rm --network host -e WEBHOOK_URL="http://127.0.0.1:$API_PORT/unknown" -e WEBHOOK_SECRET="$SECRET" -e WEBHOOK_REPOSITORY="$REPOSITORY" -e WEBHOOK_RATE=1 -e WEBHOOK_DURATION=1s -v "$ROOT/scripts/load:/scripts:ro" grafana/k6 run /scripts/webhook.js >"$ARTIFACT_DIR/control.log" 2>&1; then echo 'failing control unexpectedly passed' >&2; exit 1; fi; echo 'control PASS'
summary="$ARTIFACT_DIR/summary.json"; docker run --rm --network host -e WEBHOOK_URL="http://127.0.0.1:$API_PORT/api/v1/webhooks/github" -e WEBHOOK_SECRET="$SECRET" -e WEBHOOK_REPOSITORY="$REPOSITORY" -e WEBHOOK_RATE="${W16I_RATE:-50}" -e WEBHOOK_DURATION="${W16I_DURATION:-30s}" -v "$ROOT/scripts/load:/scripts:ro" -v "$ARTIFACT_DIR:/artifacts" grafana/k6 run --summary-export=/artifacts/summary.json /scripts/webhook.js | tee "$ARTIFACT_DIR/k6.log"
successful=$(python3 - "$summary" <<'PY'
import json,sys
s=json.load(open(sys.argv[1]));m=s['metrics'];print(int(m['http_reqs']['count'])-int(m['http_req_failed'].get('count',0)))
PY
); persisted=$(compose exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT count(*)::bigint FROM webhook_events e JOIN repositories r ON r.id=e.repository_id WHERE r.provider='github' AND r.full_name='$REPOSITORY'" | tr -d '[:space:]'); bash "$ROOT/scripts/load/reconcile-report.sh" "$summary" "$persisted" "$successful" | tee "$ARTIFACT_DIR/reconciliation.txt"
python3 - "$summary" "$ARTIFACT_DIR/report.txt" <<'PY'
import json,sys
s=json.load(open(sys.argv[1]));m=s['metrics'];v=lambda n,k='count':m[n]['values'].get(k,m[n].get(k,0));open(sys.argv[2],'w').write(f"scheduled={v('iterations')} completed={v('iterations_completed')} dropped={v('dropped_iterations')} successful={int(v('http_reqs'))-int(m['http_req_failed'].get('count',0))} non_2xx={m['http_req_failed'].get('count',0)} p95_ms={v('http_req_duration','p(95)')} actual_throughput={v('http_reqs','rate')}\n")
PY
cat "$ARTIFACT_DIR/report.txt"
