#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export JWT_SECRET=fixture-jwt REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901
project="agent-run-fixture-$(id -u)-$$"; tmp=$(mktemp -d); api_pid= observer_pid=
api_port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
pg_port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
repo=00000000-0000-0000-0000-000000000001; token=fixture-secret
cleanup(){ local rc=$?; [[ -z "$observer_pid" ]] || { kill "$observer_pid" 2>/dev/null || true; wait "$observer_pid" 2>/dev/null || true; }; [[ -z "$api_pid" ]] || { kill "$api_pid" 2>/dev/null || true; wait "$api_pid" 2>/dev/null || true; }; POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$pg_port" docker compose -p "$project" -f "$root/docker-compose.yml" down --volumes --remove-orphans >/dev/null 2>&1 || rc=1; [[ -z "$(docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.ID}}' 2>/dev/null)" ]] || rc=1; [[ -z "$(docker volume ls --filter "label=com.docker.compose.project=$project" --format '{{.Name}}' 2>/dev/null)" ]] || rc=1; rm -rf "$tmp"; exit "$rc"; }
trap cleanup EXIT INT TERM
export GOCACHE=${GOCACHE:-/private/tmp/agent-ops-gocache} GOPATH=${GOPATH:-/private/tmp/agent-ops-gopath}
(cd "$root/api" && go build -o "$tmp/agentops" ./cmd/agentops && go build -o "$tmp/api" .)
POSTGRES_PUBLISH=127.0.0.1 POSTGRES_HOST_PORT="$pg_port" docker compose -p "$project" -f "$root/docker-compose.yml" up -d postgres >/dev/null
for _ in {1..60}; do docker compose -p "$project" -f "$root/docker-compose.yml" exec -T postgres pg_isready -U agent_ops -d agent_ops >/dev/null 2>&1 && break; sleep 1; done
db="postgres://agent_ops:agent_ops_dev@127.0.0.1:$pg_port/agent_ops?sslmode=disable"
JWT_SECRET=fixture-jwt REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 DATABASE_URL="$db" HTTP_ADDR=":$api_port" SCM_REPOSITORIES_JSON='[]' AGENT_RUN_TOKEN="$token" "$tmp/api" >"$tmp/api.log" 2>&1 & api_pid=$!
for _ in {1..60}; do curl -fsS "http://127.0.0.1:$api_port/readyz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "http://127.0.0.1:$api_port/readyz" >/dev/null
docker compose -p "$project" -f "$root/docker-compose.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops -c "INSERT INTO repositories(id,provider,full_name,webhook_secret_env_ref) VALUES ('$repo','github','fixture/w9b','FIXTURE_SECRET')" >/dev/null
make_transcript(){ if [[ $1 == codex ]]; then printf '%s\n' '{"type":"thread.started","thread_id":"codex-session"}' '{"type":"turn.completed","model":"gpt-fixture","timestamp":"2026-10-27T01:00:00Z","usage":{"input_tokens":100,"cached_input_tokens":40,"cache_write_input_tokens":7,"output_tokens":20,"reasoning_output_tokens":5}}' >"$2"; else printf '%s\n' '{"type":"step_finish","timestamp":1793062800000,"sessionID":"opencode-session","part":{"tokens":{"input":100,"output":20,"reasoning":5,"cache":{"read":40,"write":7}}}}' >"$2"; fi; }
sql(){ docker compose -p "$project" -f "$root/docker-compose.yml" exec -T postgres psql -At -U agent_ops -d agent_ops -c "SELECT count(*),coalesce(min(input_tokens),0),coalesce(min(cached_input_tokens),0),coalesce(min(cache_write_tokens),0),coalesce(min(output_tokens),0),coalesce(min(reasoning_tokens),0),coalesce(min(model),''),coalesce(min(issue_numbers::text),'') FROM agent_runs WHERE repository_id='$repo' AND harness='$1'"; }
run_one(){ local h=$1 f="$tmp/$1.jsonl" expected_model=gpt-fixture; [[ $h == opencode ]] && expected_model=model-opencode; make_transcript "$h" "$f"; AGENT_RUN_TOKEN="$token" AGENTOPS_CLI="$tmp/agentops" AGENT_RUN_API_URL="http://127.0.0.1:$api_port" AGENT_RUN_REPOSITORY_ID="$repo" AGENT_RUN_ISSUES='[62,299]' AGENT_RUN_MODEL="model-$h" model="model-$h" wave=W9b worker=fixture root="$root" bash -c 'source "$0/scripts/report-agent-run.sh"; report_agent_run "$1" "$2"' "$root" "$f" "$h"; [[ "$(sql "$h")" == "1|100|40|7|20|5|$expected_model|[62, 299]" ]] || { echo "unexpected $h SQL: $(sql "$h")"; return 1; }; echo "fixture $h helper persistence passed: $(sql "$h")"; }
run_one codex; run_one opencode
run_launcher(){ local h=$1; local d="$tmp/launcher-$h"; mkdir -p "$d/scripts" "$d/.agent-runs" "$d/bin"; git -C "$d" init -q; git -C "$d" config user.email fixture@example.invalid; git -C "$d" config user.name Fixture; git -C "$d" commit --allow-empty -qm fixture; git -C "$d" branch -M main; git -C "$d" remote add origin "$d"; cp "$root/scripts/run-worker.sh" "$d/scripts/launcher.sh"; [[ $h == opencode ]] && cp "$root/scripts/run-worker-opencode.sh" "$d/scripts/launcher.sh"; cp "$root/scripts/report-agent-run.sh" "$d/scripts/report-agent-run.sh"; if [[ $h == codex ]]; then printf '#!/usr/bin/env bash\nprintf '\''%%s\\n'\'' '\''{"type":"thread.started","thread_id":"codex-session"}'\'' '\''{"type":"turn.completed","model":"gpt-fixture","timestamp":"2026-10-27T01:00:00Z","usage":{"input_tokens":100,"cached_input_tokens":40,"cache_write_input_tokens":7,"output_tokens":20,"reasoning_output_tokens":5}}'\''\n' >"$d/bin/codex"; else printf '#!/usr/bin/env bash\nprintf '\''%%s\\n'\'' '\''{"type":"step_finish","timestamp":1793062800000,"sessionID":"opencode-session","part":{"tokens":{"input":100,"output":20,"reasoning":5,"cache":{"read":40,"write":7}}}}'\'' '\''{"type":"text","part":{"text":"fixture"}}'\''\n' >"$d/bin/opencode"; fi; chmod +x "$d/bin/"*; (cd "$d" && PATH="$d/bin:$PATH" AGENTOPS_CLI="$tmp/agentops" AGENT_RUN_API_URL="http://127.0.0.1:$api_port" AGENT_RUN_TOKEN="$token" AGENT_RUN_REPOSITORY_ID="$repo" AGENT_RUN_ISSUES='[62,299]' bash scripts/launcher.sh W9b fixture-branch "$tmp/brief.md") 2>"$d/err" || { cat "$d/err"; return 1; }; [[ -f "$d/.agent-runs/W9b.jsonl" && -f "$d/.agent-runs/W9b-usage.json" ]] || return 1; echo "launcher $h passed"; }
printf 'fixture brief\n' >"$tmp/brief.md"; docker compose -p "$project" -f "$root/docker-compose.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops -c "DELETE FROM agent_runs WHERE repository_id='$repo'" >/dev/null; run_launcher codex; [[ "$(sql codex)" == '1|100|40|7|20|5|gpt-fixture|[62, 299]' ]] || { echo "launcher-created Codex SQL mismatch: $(sql codex)"; exit 1; }; run_launcher opencode; [[ "$(sql opencode)" == '1|100|40|7|20|5|openai/gpt-6-luna|[62, 299]' ]] || { echo "launcher-created OpenCode SQL mismatch: $(sql opencode)"; exit 1; }
# Verify each real launcher preserves a failing harness status even when API
# reporting fails. Use an unreachable local endpoint and identical fake output.
for h in codex opencode; do
  d="$tmp/failure-$h"; mkdir -p "$d/scripts" "$d/.agent-runs" "$d/bin"
  git -C "$d" init -q; git -C "$d" config user.email fixture@example.invalid; git -C "$d" config user.name Fixture; git -C "$d" commit --allow-empty -qm fixture; git -C "$d" remote add origin "$d"
  cp "$root/scripts/report-agent-run.sh" "$d/scripts/report-agent-run.sh"
  launcher=run-worker.sh; [[ $h == opencode ]] && launcher=run-worker-opencode.sh; cp "$root/scripts/$launcher" "$d/scripts/launcher.sh"
  if [[ $h == codex ]]; then cat >"$d/bin/codex" <<'SH'
#!/usr/bin/env bash
printf '%s\n' '{"type":"turn.completed","model":"gpt-fixture","usage":{"input_tokens":100}}'
exit 7
SH
  else cat >"$d/bin/opencode" <<'SH'
#!/usr/bin/env bash
printf '%s\n' '{"type":"step_finish","sessionID":"opencode-session","part":{"tokens":{"input":100}}}' '{"type":"text","part":{"text":"fixture"}}'
exit 7
SH
  fi
  chmod +x "$d/bin/"*; rc=0
  (cd "$d" && PATH="$d/bin:$PATH" AGENT_RUN_API_URL="http://127.0.0.1:1" AGENT_RUN_TOKEN="$token" AGENT_RUN_REPOSITORY_ID="$repo" bash scripts/launcher.sh W9b fixture-branch "$tmp/brief.md") >"$d/output" 2>&1 || rc=$?
  [[ $rc == 7 ]] || { echo "$h failed-worker status changed by reporting failure: expected 7 got $rc; output=$(<"$d/output")"; exit 1; }
  [[ $(<"$d/output") != *"$token"* ]] || { echo "$h leaked credential in reporting-failure output"; exit 1; }
echo "launcher $h HTTP failure isolation and worker exit=7 passed; credential absent from stdout/stderr"
done

# A local forwarding observer sees the actual launcher-to-API request headers
# while forwarding the unchanged request to the real API.
cat >"$tmp/observer.py" <<'PY'
import http.server, json, urllib.request, urllib.error, sys
upstream, record, port = sys.argv[1:]
class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get('Content-Length', '0')))
        token = self.headers.get('X-Agent-Run-Token')
        with open(record, 'a') as f:
            f.write(json.dumps({'token_present': token is not None, 'token_matches': token == 'fixture-secret'}) + '\n')
        req = urllib.request.Request(upstream + self.path, data=body, headers={k:v for k,v in self.headers.items() if k.lower() not in ('host','content-length')}, method='POST')
        try:
            with urllib.request.urlopen(req) as response: status, payload = response.status, response.read()
        except urllib.error.HTTPError as error: status, payload = error.code, error.read()
        self.send_response(status); self.send_header('Content-Length', str(len(payload))); self.end_headers(); self.wfile.write(payload)
    def log_message(self, *_): pass
http.server.HTTPServer(('127.0.0.1', int(port)), Handler).serve_forever()
PY
observer_port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
python3 "$tmp/observer.py" "http://127.0.0.1:$api_port" "$tmp/observed.jsonl" "$observer_port" >"$tmp/observer.log" 2>&1 & observer_pid=$!
for _ in {1..30}; do curl -s "http://127.0.0.1:$observer_port/readyz" >/dev/null 2>&1 && break; sleep .1; done
printf 'fixture brief\n' >"$tmp/brief.md"
docker compose -p "$project" -f "$root/docker-compose.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U agent_ops -d agent_ops -c 'DELETE FROM agent_runs' >/dev/null
for h in codex opencode; do
  d="$tmp/launcher-$h"; : >"$tmp/observed.jsonl"
  (cd "$d" && PATH="$d/bin:$PATH" AGENTOPS_CLI="$tmp/agentops" AGENT_RUN_API_URL="http://127.0.0.1:$observer_port" AGENT_RUN_TOKEN="$token" AGENT_RUN_REPOSITORY_ID="$repo" AGENT_RUN_ISSUES='[62,299]' bash scripts/launcher.sh W9b fixture-branch "$tmp/brief.md") >"$tmp/$h.stdout" 2>"$tmp/$h.stderr"
  python3 - "$tmp/observed.jsonl" <<'PY'
import json,sys
rows=[json.loads(line) for line in open(sys.argv[1])]
if len(rows)!=1 or rows[0] != {'token_present':True,'token_matches':True}: raise SystemExit('launcher credential-header assertion failed')
PY
  ! grep -q "$token" "$tmp/$h.stdout" "$tmp/$h.stderr" || { echo "$h leaked credential"; exit 1; }
  [[ "$(sql "$h")" == "1|100|40|7|20|5|$( [[ $h == opencode ]] && printf openai/gpt-6-luna || printf gpt-fixture )|[62, 299]" ]] || { echo "$h observed request did not persist expected SQL"; exit 1; }
  echo "launcher $h observed X-Agent-Run-Token and API persisted expected SQL; credential absent from output"
done
control_root="$tmp/production-control"; mkdir -p "$control_root/api/cmd/agentops"; cp -R "$root/api/." "$control_root/api/"; python3 - "$control_root/api/cmd/agentops/main.go" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]); s=p.read_text(); old='"session_id": session.ID'
if old not in s: raise SystemExit('production mutation target missing')
p.write_text(s.replace(old, '"session_id": "mutated-session"', 1))
PY
(cd "$control_root/api" && go build -o "$tmp/agentops-mutated" ./cmd/agentops)
control="$tmp/launcher-codex/.agent-runs/W9b.jsonl"
set +e
AGENT_RUN_TOKEN="$token" "$tmp/agentops-mutated" push --harness codex --endpoint "http://127.0.0.1:$api_port/api/v1/agent-runs" --repository-id "$repo" --wave W9b --worker fixture --issue-numbers '[62,299]' "$control" >"$tmp/control.stdout" 2>"$tmp/control.stderr"
control_rc=$?
set -e
control_diag=$(sql codex)
if [[ "$control_diag" == "1|100|40|7|20|5|gpt-fixture|[62, 299]" ]]; then echo 'production regression control passed unexpectedly with identical Codex transcript'; exit 1; fi
[[ $control_rc == 0 ]] || { echo "mutated CLI unexpectedly failed before SQL assertion (exit=$control_rc): $(<"$tmp/control.stderr")"; exit 1; }
[[ "$control_diag" == "2|100|40|7|20|5|gpt-fixture|[62, 299]" ]] || { echo "unexpected production-control SQL: $control_diag"; exit 1; }
echo "ordinary SQL assertion failed for production mutation: expected one unchanged Codex row, got $control_diag"
AGENT_RUN_TOKEN="$token" "$tmp/agentops" push --harness codex --endpoint "http://127.0.0.1:$api_port/api/v1/agent-runs" --repository-id "$repo" --wave W9b --worker fixture --issue-numbers '[62,299]' "$control"
[[ "$(sql codex)" == "2|100|40|7|20|5|gpt-fixture|[62, 299]" ]] || { echo 'restored production replay changed row count'; exit 1; }
echo "restored pass with identical inputs: $(sql codex)"
set +e
AGENT_RUN_TOKEN=wrong-fixture-secret "$tmp/agentops" push --harness codex --endpoint "http://127.0.0.1:$api_port/api/v1/agent-runs" --repository-id "$repo" --wave W9b --worker fixture --issue-numbers '[62,299]' "$control" >"$tmp/wrong-token.stdout" 2>"$tmp/wrong-token.stderr"
wrong_token_rc=$?
set -e
[[ $wrong_token_rc != 0 ]] || { echo 'wrong credential unexpectedly accepted'; exit 1; }
! grep -q 'wrong-fixture-secret' "$tmp/wrong-token.stdout" "$tmp/wrong-token.stderr" || { echo 'wrong credential leaked to CLI output'; exit 1; }
wrong_status=$(curl -sS -o "$tmp/wrong-http.body" -w '%{http_code}' -X POST "http://127.0.0.1:$api_port/api/v1/agent-runs" -H 'Content-Type: application/json' -H 'X-Agent-Run-Token: wrong-fixture-secret' --data '{"repository_id":"00000000-0000-0000-0000-000000000001","harness":"codex"}')
[[ $wrong_status == 401 || $wrong_status == 403 ]] || { echo "wrong credential HTTP status was $wrong_status"; exit 1; }
[[ "$(sql codex)" == '2|100|40|7|20|5|gpt-fixture|[62, 299]' ]] || { echo 'wrong-token request changed SQL'; exit 1; }
echo "wrong-token API request rejected HTTP $wrong_status; unchanged SQL; CLI exit=$wrong_token_rc; credential absent from stdout/stderr"
set +e
"$tmp/agentops" push --harness codex --endpoint "http://127.0.0.1:$api_port/api/v1/agent-runs" --repository-id "$repo" --wave W9b --worker fixture --issue-numbers '[62,299]' "$control" >"$tmp/missing-token.stdout" 2>"$tmp/missing-token.stderr"
missing_rc=$?
set -e
[[ $missing_rc != 0 ]] || { echo 'missing credential unexpectedly accepted'; exit 1; }
! grep -q 'fixture-secret' "$tmp/missing-token.stdout" "$tmp/missing-token.stderr" || { echo 'credential leaked'; exit 1; }
missing_status=$(curl -sS -o "$tmp/missing-http.body" -w '%{http_code}' -X POST "http://127.0.0.1:$api_port/api/v1/agent-runs" -H 'Content-Type: application/json' --data '{"repository_id":"00000000-0000-0000-0000-000000000001","harness":"codex"}')
[[ $missing_status == 401 || $missing_status == 403 ]] || { echo "missing credential HTTP status was $missing_status"; exit 1; }
[[ "$(sql codex)" == '2|100|40|7|20|5|gpt-fixture|[62, 299]' ]] || { echo 'missing-token request changed SQL'; exit 1; }
echo "missing-token API request rejected HTTP $missing_status; unchanged SQL; CLI exit=$missing_rc"
AGENT_RUN_API_URL=http://127.0.0.1 AGENT_RUN_TOKEN= AGENT_RUN_REPOSITORY_ID="$repo" root="$root" bash -c 'source "$0/scripts/report-agent-run.sh"; report_agent_run "$1" codex' "$root" "$tmp/no-such-transcript"
echo 'fixture opt-out passed; real CLI and disposable API/Postgres replay proof passed'
# Keep this helper executable from CI and developer shells.
