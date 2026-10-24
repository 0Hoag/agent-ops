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
        token = self.headers.get("X-Agent-Run-Token", "")
        expected_token = open(os.environ["FIXTURE_EXPECTED_TOKEN"]).read()
        if token != expected_token:
            with open(os.environ["FIXTURE_CREDENTIAL_ERRORS"], "a") as responses:
                responses.write(f"credential header mismatch: expected {expected_token!r}, got {token!r}\n")
            self.send_error(400, "credential header mismatch")
            return
        body = self.rfile.read(int(self.headers["Content-Length"]))
        open(os.environ["FIXTURE_BODY"], "wb").write(body)
        status = 503 if open(os.environ["FIXTURE_MODE"]).read().strip() == "1" else 201
        with open(os.environ["FIXTURE_RESPONSES"], "a") as responses:
            responses.write(f"{status}\n")
        self.send_response(status)
        self.end_headers()
        self.wfile.write(b"fixture-secret transcript")
    def log_message(self, *_): pass
HTTPServer(("127.0.0.1", int(os.environ["FIXTURE_PORT"])), Handler).serve_forever()
PY
export FIXTURE_PORT=$((20000 + $$ % 20000)) FIXTURE_BODY="$tmp/request.json" FIXTURE_RESPONSES="$tmp/responses.log" FIXTURE_MODE="$tmp/mode" FIXTURE_EXPECTED_TOKEN="$tmp/expected-token" FIXTURE_CREDENTIAL_ERRORS="$tmp/credential-errors.log"
touch "$FIXTURE_RESPONSES"
touch "$FIXTURE_CREDENTIAL_ERRORS"
printf '0' > "$FIXTURE_MODE"
python3 "$tmp/server.py" > "$tmp/server.log" 2>&1 & server_pid=$!
for _ in {1..50}; do if python3 -c 'import socket,sys; s=socket.socket(); s.settimeout(.1); r=s.connect_ex(("127.0.0.1",int(sys.argv[1]))); s.close(); sys.exit(r)' "$FIXTURE_PORT"; then break; fi; sleep .05; done

run_fixture() {
  local harness=$1 expected_status=$2 reject=${3:-0} worker_status=${4:-0}
  printf 'fixture-secret' > "$FIXTURE_EXPECTED_TOKEN"
  printf '%s' "$reject" > "$FIXTURE_MODE"
  : > "$FIXTURE_RESPONSES"
  : > "$FIXTURE_CREDENTIAL_ERRORS"
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
  assert_http_result() {
    local status=$1 expected_status=$2 runroot=$3 output=$4 reject=$5
    [[ $status == $expected_status ]] || { printf 'worker status mismatch: %s\n' "$status"; cat "$output"; return 1; }
    if [[ -s $FIXTURE_EXPECTED_TOKEN ]]; then
      [[ ! -s $FIXTURE_CREDENTIAL_ERRORS ]] || { printf '%s\n' "$(<"$FIXTURE_CREDENTIAL_ERRORS")"; return 1; }
      [[ $(wc -l < "$FIXTURE_RESPONSES") -eq 1 ]] || { echo 'credential header assertion missing request'; return 1; }
      [[ $(<"$FIXTURE_RESPONSES") == 201 || $(<"$FIXTURE_RESPONSES") == 503 ]] || { echo 'credential header assertion request failed'; return 1; }
    fi
    if (( reject != 0 )); then
      grep -q '"input_tokens":100' "$tmp/request.json" || { echo 'HTTP rejection fixture request missing'; return 1; }
      [[ $(wc -l < "$FIXTURE_RESPONSES") -eq 1 && $(<"$FIXTURE_RESPONSES") == 503 ]] || { echo 'HTTP rejection server did not return 503'; return 1; }
    fi
    [[ -f $runroot/.agent-runs/W3b.jsonl && -f $runroot/.agent-runs/W3b-usage.json ]] || { echo 'worker JSONL/usage summary missing'; return 1; }
    [[ $harness != opencode || -f $runroot/.agent-runs/W3b.md ]] || { echo 'OpenCode final report missing'; return 1; }
  }
  if (( reject != 0 )); then
    # Keep control execution on the common assertion below; capture the failure
    # for the outer proof while retaining observed request evidence.
    set +e
    assert_http_result "$status" "$expected_status" "$runroot" "$tmp/$harness.out" "$reject"
    local assertion_status=$?
    set -e
    [[ $assertion_status == 0 ]] || return "$assertion_status"
  else
    assert_http_result "$status" "$expected_status" "$runroot" "$tmp/$harness.out" "$reject"
  fi
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
    :
  fi
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

# Disposable HTTP-failure controls: leak the observed HTTP 503 through a
# mutated launcher and require the ordinary status/artifact assertion to fail.
for harness in codex opencode; do
  control="$tmp/reject-control-$harness"
  mkdir -p "$control/scripts" "$control/.agent-runs" "$control/../$(basename "$control")-wt"
  cp "$root/scripts/report-agent-run.sh" "$control/scripts/report-agent-run.sh"
  git -C "$control" init -q; git -C "$control" config user.email fixture@example.invalid; git -C "$control" config user.name Fixture; git -C "$control" commit --allow-empty -qm fixture; git -C "$control" remote add origin "$tmp/worktree"
  ln -sfn "$tmp/worktree" "$control/../$(basename "$control")-wt/fixture-branch"
  launcher=run-worker.sh
  [[ $harness == opencode ]] && launcher=run-worker-opencode.sh
  cp "$root/scripts/$launcher" "$control/scripts/launcher.sh"
  # The disposable launcher exits on report failure instead of preserving its
  # worker's status. Use a top-level exit, not return (invalid at top level).
  python3 - "$control/scripts/launcher.sh" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]); s=p.read_text()
needle='report_agent_run "$usage" "$started_at" "$ended_at" || printf'
if needle not in s: raise SystemExit('HTTP launcher mutation target not found')
p.write_text(s.replace(needle, 'report_agent_run "$usage" "$started_at" "$ended_at" || exit $? # || printf', 1))
PY
  printf '1' > "$FIXTURE_MODE"; : > "$FIXTURE_RESPONSES"; : > "$FIXTURE_CREDENTIAL_ERRORS"; printf 'fixture-secret' > "$FIXTURE_EXPECTED_TOKEN"
  set +e
  (cd "$control" && PATH="$tmp/bin:$PATH" FIXTURE_BODY="$tmp/request.json" FIXTURE_WORKER_STATUS=0 \
    AGENT_RUN_API_URL="http://127.0.0.1:$FIXTURE_PORT" AGENT_RUN_TOKEN=fixture-secret \
    AGENT_RUN_REPOSITORY_ID=00000000-0000-0000-0000-000000000001 \
    bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/reject-control-$harness.out" 2>&1
  control_status=$?
  set -e
  [[ $(wc -l < "$FIXTURE_RESPONSES") -eq 1 && $(<"$FIXTURE_RESPONSES") == 503 ]] || { echo "$harness rejection mutation did not reach fake server"; exit 1; }
  set +e
  assert_http_result "$control_status" 0 "$control" "$tmp/reject-control-$harness.out" 1 > "$tmp/reject-assert-$harness.out" 2>&1
  assertion_status=$?
  set -e
  [[ $assertion_status != 0 && $control_status != 0 && $(<"$tmp/reject-assert-$harness.out") == *'worker status mismatch'* ]] || { echo "$harness rejection control did not fail unchanged worker-status assertion"; cat "$tmp/reject-assert-$harness.out"; exit 1; }
  echo "fixture $harness rejection mutation: unchanged worker-status assertion failed: $(<"$tmp/reject-assert-$harness.out")"
done
for harness in codex opencode; do
  control="$tmp/credential-control-$harness"
  mkdir -p "$control/scripts" "$control/.agent-runs" "$control/../$(basename "$control")-wt"
  cp "$root/scripts/report-agent-run.sh" "$control/scripts/report-agent-run.sh"
  git -C "$control" init -q; git -C "$control" config user.email fixture@example.invalid; git -C "$control" config user.name Fixture; git -C "$control" commit --allow-empty -qm fixture; git -C "$control" remote add origin "$tmp/worktree"
  ln -sfn "$tmp/worktree" "$control/../$(basename "$control")-wt/fixture-branch"
  launcher=run-worker.sh
  [[ $harness == opencode ]] && launcher=run-worker-opencode.sh
  cp "$root/scripts/$launcher" "$control/scripts/launcher.sh"
  printf 'wrong-token' > "$FIXTURE_EXPECTED_TOKEN"
  : > "$FIXTURE_CREDENTIAL_ERRORS"; : > "$FIXTURE_RESPONSES"; printf '0' > "$FIXTURE_MODE"
  set +e
  (cd "$control" && PATH="$tmp/bin:$PATH" FIXTURE_BODY="$tmp/request.json" FIXTURE_WORKER_STATUS=0 \
    AGENT_RUN_API_URL="http://127.0.0.1:$FIXTURE_PORT" AGENT_RUN_TOKEN=fixture-secret \
    AGENT_RUN_REPOSITORY_ID=00000000-0000-0000-0000-000000000001 \
    bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/credential-control-$harness.out" 2>&1
  control_status=$?
  set -e
  diagnostic=$(<"$FIXTURE_CREDENTIAL_ERRORS")
  [[ $control_status == 0 && $diagnostic == *'credential header mismatch: expected '\''wrong-token'\'', got '\''fixture-secret'\'''* ]] || { echo "$harness credential mutation failed its same credential assertion: $diagnostic"; exit 1; }
  echo "fixture $harness credential mutation: $diagnostic"
done
printf 'fixture-secret' > "$FIXTURE_EXPECTED_TOKEN"
printf '0' > "$FIXTURE_MODE"

# Opt-out baseline and controls use the same fake worker output and inputs.
unset FIXTURE_BODY
: > "$FIXTURE_RESPONSES"
response_count_before=$(wc -l < "$FIXTURE_RESPONSES")
cat > "$tmp/bin/codex" <<'SH'
#!/usr/bin/env bash
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":100}}'
exit 0
SH
cat > "$tmp/bin/opencode" <<'SH'
#!/usr/bin/env bash
printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":100}}}' '{"type":"text","part":{"text":"fixture final"}}'
exit 0
SH
chmod +x "$tmp/bin/codex" "$tmp/bin/opencode"
noapi="$tmp/noapi"
mkdir -p "$noapi/scripts" "$noapi/.agent-runs" "$noapi/../$(basename "$noapi")-wt"
cp "$root/scripts/run-worker.sh" "$noapi/scripts/launcher.sh"
cp "$root/scripts/report-agent-run.sh" "$noapi/scripts/report-agent-run.sh"
git -C "$noapi" init -q; git -C "$noapi" config user.email fixture@example.invalid; git -C "$noapi" config user.name Fixture; git -C "$noapi" commit --allow-empty -qm fixture
mkdir -p "$noapi/../$(basename "$noapi")-wt"
ln -sfn "$tmp/worktree" "$noapi/../$(basename "$noapi")-wt/fixture-branch"
git -C "$noapi" remote add origin "$tmp/worktree"
run_optout_assertion() {
  local actual=$1
  [[ $actual -eq $response_count_before ]] || { echo "opt-out unexpectedly sent a request (baseline=$response_count_before actual=$actual)"; return 1; }
}
run_optout() {
local harness=$1 runroot=$2
(cd "$runroot" && PATH="$tmp/bin:$PATH" FIXTURE_BODY="$tmp/request.json" FIXTURE_WORKER_STATUS=0 \
  AGENT_RUN_API_URL="http://127.0.0.1:$FIXTURE_PORT" AGENT_RUN_TOKEN='' \
  AGENT_RUN_REPOSITORY_ID=00000000-0000-0000-0000-000000000001 \
  bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/optout-$harness.out" 2>&1
  local requests=$(wc -l < "$FIXTURE_RESPONSES" | tr -d ' ')
  run_optout_assertion "$requests"
}
for harness in codex opencode; do
  runroot="$tmp/noapi-$harness"
  mkdir -p "$runroot/scripts" "$runroot/.agent-runs" "$runroot/../$(basename "$runroot")-wt"
  cp "$root/scripts/$([[ $harness == codex ]] && printf run-worker.sh || printf run-worker-opencode.sh)" "$runroot/scripts/launcher.sh"
  cp "$root/scripts/report-agent-run.sh" "$runroot/scripts/report-agent-run.sh"
  git -C "$runroot" init -q; git -C "$runroot" config user.email fixture@example.invalid; git -C "$runroot" config user.name Fixture; git -C "$runroot" commit --allow-empty -qm fixture; git -C "$runroot" remote add origin "$tmp/worktree"
  ln -sfn "$tmp/worktree" "$runroot/../$(basename "$runroot")-wt/fixture-branch"
  run_optout "$harness" "$runroot"
  [[ -f $runroot/.agent-runs/W3b.jsonl && -f $runroot/.agent-runs/W3b-usage.json ]] || { echo 'empty-usage local output missing'; exit 1; }
  python3 - "$runroot/.agent-runs/W3b-usage.json" <<'PY'
import json,sys
u=json.load(open(sys.argv[1])); assert u.get("input")==100,u
PY
done
echo 'fixture opt-out passed'

# Both harnesses must reject bypassing the actual reporter opt-out guard.
# Identical inputs make the mutated guard send the empty-token request.
for harness in codex opencode; do
  control="$tmp/optout-control-$harness"
  mkdir -p "$control/scripts" "$control/.agent-runs" "$control/../$(basename "$control")-wt"
  cp "$root/scripts/report-agent-run.sh" "$control/scripts/report-agent-run.sh"
  git -C "$control" init -q; git -C "$control" config user.email fixture@example.invalid; git -C "$control" config user.name Fixture; git -C "$control" commit --allow-empty -qm fixture; git -C "$control" remote add origin "$tmp/worktree"
  ln -sfn "$tmp/worktree" "$control/../$(basename "$control")-wt/fixture-branch"
  launcher=run-worker.sh
  [[ $harness == opencode ]] && launcher=run-worker-opencode.sh
  cp "$root/scripts/$launcher" "$control/scripts/launcher.sh"
  # Mutate only the disposable reporter guard; leave the empty token untouched.
  python3 - "$root/scripts/report-agent-run.sh" "$control/scripts/report-agent-run.sh" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[2]); s=pathlib.Path(sys.argv[1]).read_text()
needle='[[ -n ${AGENT_RUN_API_URL:-} && -n ${AGENT_RUN_TOKEN:-} && -n ${AGENT_RUN_REPOSITORY_ID:-} ]] || return 0'
if needle not in s: raise SystemExit('opt-out reporter guard mutation target not found')
replacement='[[ -n ${AGENT_RUN_API_URL:-} && -n ${AGENT_RUN_REPOSITORY_ID:-} ]] || return 0'
p.write_text(s.replace(needle, replacement, 1))
PY
  : > "$FIXTURE_RESPONSES"; : > "$FIXTURE_CREDENTIAL_ERRORS"; printf '0' > "$FIXTURE_MODE"; : > "$FIXTURE_EXPECTED_TOKEN"
  set +e
  (cd "$control" && PATH="$tmp/bin:$PATH" FIXTURE_BODY="$tmp/request.json" FIXTURE_WORKER_STATUS=0 \
    AGENT_RUN_API_URL="http://127.0.0.1:$FIXTURE_PORT" \
    AGENT_RUN_TOKEN='' \
    AGENT_RUN_REPOSITORY_ID=00000000-0000-0000-0000-000000000001 \
    bash scripts/launcher.sh W3b fixture-branch "$tmp/brief.md") > "$tmp/optout-control-$harness.out" 2>&1
  control_status=$?
  set -e
  grep -q '"input_tokens":100' "$tmp/request.json" || { echo "$harness opt-out mutation did not submit a report payload"; exit 1; }
  [[ $control_status == 0 ]] || { echo "$harness opt-out mutation failed before assertion (exit=$control_status)"; cat "$tmp/optout-control-$harness.out"; exit 1; }
  mutation_count=$(wc -l < "$FIXTURE_RESPONSES" | tr -d ' ')
  [[ $mutation_count -eq 1 ]] || { echo "$harness opt-out mutation did not reach fake server exactly once (count=$mutation_count)"; cat "$tmp/optout-control-$harness.out"; cat "$tmp/server.log"; exit 1; }
  set +e
  run_optout_assertion "$mutation_count" > "$tmp/optout-assert-$harness.out" 2>&1
  assertion_status=$?
  set -e
  [[ $assertion_status != 0 && $(<"$tmp/optout-assert-$harness.out") == *'opt-out unexpectedly sent a request'* ]] || { echo "$harness unchanged opt-out assertion did not fail with request diagnostic"; cat "$tmp/optout-assert-$harness.out"; exit 1; }
  echo "fixture $harness opt-out mutation: unchanged request-count assertion failed: $(<"$tmp/optout-assert-$harness.out")"
done
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
