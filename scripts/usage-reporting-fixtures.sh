#!/usr/bin/env bash
set -euo pipefail

root=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
server_pid=
cleanup() {
  if [[ -n $server_pid ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; server_pid=; fi
  rm -rf "$tmp"
}
trap cleanup EXIT
mkdir -p "$tmp/bin" "$tmp/worktree"
git -C "$tmp/worktree" init -q
git -C "$tmp/worktree" config user.email fixture@example.invalid
git -C "$tmp/worktree" config user.name Fixture
git -C "$tmp/worktree" commit --allow-empty -qm fixture
printf 'fixture brief\n' > "$tmp/brief.md"

cat > "$tmp/bin/codex" <<'SH'
#!/usr/bin/env bash
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"cache_write_input_tokens":7,"output_tokens":20,"reasoning_output_tokens":5}}'
exit "${FIXTURE_WORKER_STATUS:-0}"
SH
cat > "$tmp/bin/opencode" <<'SH'
#!/usr/bin/env bash
printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":100,"output":20,"reasoning":5,"cache":{"read":40,"write":7}}}}' '{"type":"text","part":{"text":"fixture final"}}'
exit "${FIXTURE_WORKER_STATUS:-0}"
SH
chmod +x "$tmp/bin/"*

cat > "$tmp/server.py" <<'PY'
from http.server import BaseHTTPRequestHandler, HTTPServer
import json, os
class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        assert self.path == "/api/v1/agent-runs", self.path
        assert self.headers.get("X-Agent-Run-Token") == "fixture-secret"
        body = self.rfile.read(int(self.headers["Content-Length"]))
        open(os.environ["FIXTURE_BODY"], "wb").write(body)
        self.send_response(503 if os.environ.get("FIXTURE_HTTP_REJECT") == "1" else 201)
        self.end_headers()
        self.wfile.write(b"fixture-secret transcript")
    def log_message(self, *_): pass
HTTPServer(("127.0.0.1", int(os.environ["FIXTURE_PORT"])), Handler).serve_forever()
PY
export FIXTURE_PORT=$((20000 + $$ % 20000)) FIXTURE_BODY="$tmp/request.json"
python3 "$tmp/server.py" > "$tmp/server.log" 2>&1 & server_pid=$!
for _ in {1..50}; do if python3 -c 'import socket,sys; s=socket.socket(); s.settimeout(.1); r=s.connect_ex(("127.0.0.1",int(sys.argv[1]))); s.close(); sys.exit(r)' "$FIXTURE_PORT"; then break; fi; sleep .05; done

run_fixture() {
  local harness=$1 expected_status=$2 reject=${3:-0} worker_status=${4:-0}
  local runroot="$tmp/${harness:-codex}-$reject-$worker_status"
  mkdir -p "$runroot/scripts" "$runroot/.agent-runs" "$runroot/../$(basename "$runroot")-wt"
  cp "$root/scripts/report-agent-run.sh" "$runroot/scripts/report-agent-run.sh"
  git -C "$runroot" init -q
  git -C "$runroot" config user.email fixture@example.invalid
  git -C "$runroot" config user.name Fixture
  git -C "$runroot" commit --allow-empty -qm fixture
  git -C "$runroot" remote add origin "$tmp/worktree"
  # Launchers calculate a sibling worktree path; link the fixture worktree at that path.
  mkdir -p "$runroot/../$(basename "$runroot")-wt"
  ln -sfn "$tmp/worktree" "$runroot/../$(basename "$runroot")-wt/fixture-branch" 2>/dev/null || true
  local launcher=run-worker.sh
  [[ $harness == opencode ]] && launcher=run-worker-opencode.sh
  cp "$root/scripts/$launcher" "$runroot/scripts/launcher.sh"
  set +e
  (cd "$runroot" && PATH="$tmp/bin:$PATH" FIXTURE_BODY="$tmp/request.json" FIXTURE_WORKER_STATUS="$worker_status" FIXTURE_HTTP_REJECT="$reject" \
    AGENT_RUN_API_URL="http://127.0.0.1:$FIXTURE_PORT" AGENT_RUN_TOKEN=fixture-secret \
    AGENT_RUN_REPOSITORY_ID=00000000-0000-0000-0000-000000000001 AGENT_RUN_ISSUES='[28,29]' \
    bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/$harness.out" 2>&1
  local status=$?
  set -e
  [[ $status == $expected_status ]] || { printf 'worker status mismatch: %s\n' "$status"; cat "$tmp/$harness.out"; return 1; }
  if (( worker_status != 0 )); then
    grep -q '"input_tokens":100' "$tmp/request.json" || { echo 'failed-worker report missing'; return 1; }
  fi
  if (( reject == 0 )); then
    if [[ $worker_status == 0 ]]; then python3 - "$tmp/request.json" "$harness" <<'PY'
import json,sys
p=json.load(open(sys.argv[1])); h=sys.argv[2]
expected={"repository_id":"00000000-0000-0000-0000-000000000001","wave":"W3b","worker":"codex" if h=="codex" else "opencode","model":"gpt-reserve" if h=="codex" else "openai/gpt-6-luna","issue_numbers":[28,29],"input_tokens":100,"cached_input_tokens":40,"cache_write_tokens":7,"output_tokens":20,"reasoning_tokens":5}
assert all(p.get(k)==v for k,v in expected.items()), (p,expected)
assert "fixture-secret" not in json.dumps(p) and "transcript" not in json.dumps(p)
PY
    fi
  else
    grep -q '"input_tokens":100' "$tmp/request.json" || { echo 'HTTP rejection fixture request missing'; return 1; }
  fi
  [[ -f $runroot/.agent-runs/W3b.jsonl && -f $runroot/.agent-runs/W3b-usage.json && -f $runroot/.agent-runs/W3b.md ]] || { echo 'worker local JSONL/summary/final report missing'; return 1; }
  [[ $harness != opencode || -f $runroot/.agent-runs/W3b.md ]] || { echo 'OpenCode summary missing'; return 1; }
  ! grep -Eq 'fixture-secret|transcript' "$runroot/.agent-runs/W3b.jsonl" "$tmp/$harness.out"
  if [[ $harness == opencode ]]; then ! grep -Eq 'fixture-secret|transcript' "$runroot/.agent-runs/W3b.md"; fi
  printf 'fixture %s reject=%s worker=%s exit=%s passed\n' "${harness:-codex}" "$reject" "$worker_status" "$status"
}

run_fixture codex 0
run_fixture opencode 0
run_fixture codex 7 0 7
run_fixture opencode 7 0 7
run_fixture codex 0 1 0
run_fixture opencode 0 1 0

# Opt-out must not make a request but retains local artifacts.
unset FIXTURE_BODY
cat > "$tmp/bin/codex" <<'SH'
#!/usr/bin/env bash
exit 0
SH
chmod +x "$tmp/bin/codex"
noapi="$tmp/noapi"
mkdir -p "$noapi/scripts" "$noapi/.agent-runs" "$noapi/../$(basename "$noapi")-wt"
cp "$root/scripts/run-worker.sh" "$noapi/scripts/launcher.sh"
cp "$root/scripts/report-agent-run.sh" "$noapi/scripts/report-agent-run.sh"
git -C "$noapi" init -q; git -C "$noapi" config user.email fixture@example.invalid; git -C "$noapi" config user.name Fixture; git -C "$noapi" commit --allow-empty -qm fixture
mkdir -p "$noapi/../$(basename "$noapi")-wt"
ln -sfn "$tmp/worktree" "$noapi/../$(basename "$noapi")-wt/fixture-branch"
git -C "$noapi" remote add origin "$tmp/worktree"
(cd "$noapi" && PATH="$tmp/bin:$PATH" FIXTURE_WORKER_STATUS=0 env -u AGENT_RUN_API_URL -u AGENT_RUN_TOKEN -u AGENT_RUN_REPOSITORY_ID \
  bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/noapi.out" 2>&1
[[ -f $noapi/.agent-runs/W3b.jsonl && -f $noapi/.agent-runs/W3b-usage.json ]] || { echo 'empty-usage local output missing'; exit 1; }
rm -f "$tmp/request.json"
python3 - "$noapi/.agent-runs/W3b-usage.json" <<'PY'
import json,sys
u=json.load(open(sys.argv[1])); assert all(u.get(k)==0 for k in ("input","cached_input","cache_write","output","reasoning")),u
PY
echo 'fixture opt-out passed'
[[ ! -e $tmp/request.json ]] || { echo 'opt-out unexpectedly sent a request'; exit 1; }

# No-usage run still reports a zero-valued API record and keeps local artifacts.
zero="$tmp/zero"
mkdir -p "$zero/scripts" "$zero/.agent-runs" "$zero/../$(basename "$zero")-wt"
cp "$root/scripts/run-worker.sh" "$zero/scripts/launcher.sh"
cp "$root/scripts/report-agent-run.sh" "$zero/scripts/report-agent-run.sh"
git -C "$zero" init -q; git -C "$zero" config user.email fixture@example.invalid; git -C "$zero" config user.name Fixture; git -C "$zero" commit --allow-empty -qm fixture; git -C "$zero" remote add origin "$tmp/worktree"
ln -sfn "$tmp/worktree" "$zero/../$(basename "$zero")-wt/fixture-branch"
cat > "$tmp/bin/codex" <<'SH'
#!/usr/bin/env bash
exit 0
SH
chmod +x "$tmp/bin/codex"
(cd "$zero" && PATH="$tmp/bin:$PATH" FIXTURE_BODY="$tmp/request.json" FIXTURE_WORKER_STATUS=0 FIXTURE_HTTP_REJECT=0 \
 AGENT_RUN_API_URL="http://127.0.0.1:$FIXTURE_PORT" AGENT_RUN_TOKEN=fixture-secret AGENT_RUN_REPOSITORY_ID=00000000-0000-0000-0000-000000000001 \
 bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/zero.out" 2>&1
python3 - "$tmp/request.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1])); assert all(p[k]==0 for k in ("input_tokens","cached_input_tokens","cache_write_tokens","output_tokens","reasoning_tokens")),p
PY
[[ -f $zero/.agent-runs/W3b.jsonl && -f $zero/.agent-runs/W3b-usage.json ]] || { echo 'zero-usage local output missing'; exit 1; }
echo 'fixture empty usage passed'

# The exact-payload assertion must reject a normalization regression.
cat > "$tmp/bin/codex" <<'SH'
#!/usr/bin/env bash
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"cache_write_input_tokens":7,"output_tokens":20,"reasoning_output_tokens":5}}'
SH
chmod +x "$tmp/bin/codex"
control="$tmp/control"
mkdir -p "$control/scripts" "$control/.agent-runs" "$control/../$(basename "$control")-wt"
cp "$root/scripts/run-worker.sh" "$control/scripts/launcher.sh"
cp "$root/scripts/report-agent-run.sh" "$control/scripts/report-agent-run.sh"
git -C "$control" init -q; git -C "$control" config user.email fixture@example.invalid; git -C "$control" config user.name Fixture; git -C "$control" commit --allow-empty -qm fixture; git -C "$control" remote add origin "$tmp/worktree"
ln -s "$tmp/worktree" "$control/../$(basename "$control")-wt/fixture-branch"
sed 's/\[\$u\[\].input_tokens/[$u[].output_tokens/' "$root/scripts/run-worker.sh" > "$tmp/mutated-launcher.sh"
cp "$tmp/mutated-launcher.sh" "$control/scripts/launcher.sh"
set +e
(cd "$control" && PATH="$tmp/bin:$PATH" FIXTURE_BODY="$tmp/request.json" FIXTURE_WORKER_STATUS=0 FIXTURE_HTTP_REJECT=0 \
 AGENT_RUN_API_URL="http://127.0.0.1:$FIXTURE_PORT" AGENT_RUN_TOKEN=fixture-secret AGENT_RUN_REPOSITORY_ID=00000000-0000-0000-0000-000000000001 AGENT_RUN_ISSUES='[28,29]' \
 bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/control.out" 2>&1
control_status=$?
set -e
control_diagnostic=$(python3 - "$tmp/request.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1])); expected=100
if p.get("input_tokens") != expected:
    print(f"payload mismatch: input_tokens expected {expected}, got {p.get('input_tokens')}")
    sys.exit(1)
PY
 2>&1)
[[ $control_status == 0 && $control_diagnostic == *'payload mismatch: input_tokens expected 100, got 20'* ]] || { printf 'normalization control failed unexpectedly (exit=%s): %s\n' "$control_status" "$control_diagnostic"; exit 1; }
echo "$control_diagnostic"
echo 'normalization control: exact-payload assertion rejected mutated input token normalization (expected input_tokens=100)'
