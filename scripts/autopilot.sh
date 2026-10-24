#!/usr/bin/env bash
# Coordinator loop: the shell waits, the model only works.
# Each pass is a fresh, short OpenCode session (`/autopilot` = one pass of AGENTS.md → Autopilot), then
# the shell sleeps. Workers run detached (scripts/spawn.sh) and outlive the pass.
#
#   scripts/autopilot.sh                 # pass every 10 min, or within 30 s after a worker ends
#   INTERVAL=300 scripts/autopilot.sh    # every 5 min
#   CAP=2 scripts/autopilot.sh           # with CAP workers running, timer passes are skipped (3 x INTERVAL max)
#   touch .agent-runs/STOP               # stop after the current pass
#
# If the OpenCode web server is up (scripts/opencode.sh web --port 4096), each pass runs on it
# (`--attach`) and shows live in the web UI; otherwise it runs locally. Override with WEB=<url>.
#
# Per pass: .agent-runs/coordinator/<time>.jsonl (events) and .md (final message).
# A final message whose first line starts with "AUTOPILOT: STOP" ends the loop.
set -uo pipefail

cd "$(git rev-parse --show-toplevel)"
interval=${INTERVAL:-600}
poll=${POLL:-30}
cap=${CAP:-2}   # keep equal to the worker cap in AGENTS.md → Dispatch
web=${WEB:-http://127.0.0.1:4096}
dir=.agent-runs/coordinator
mkdir -p "$dir"
rm -f .agent-runs/STOP

# Local-only checks (no network, no tokens): running worker pids and the final-message files
# (.agent-runs/<wave>.md) that run-worker*.sh writes when a worker ends.
worker_pids() { pgrep -f 'scripts/run-worker' | sort | tr '\n' ' '; }
results_stamp() { stat -f '%N %m' .agent-runs/*.md 2>/dev/null | shasum; }

# Sleep up to $interval, but wake after at most $poll seconds once a worker ends or writes its result.
# While $cap workers run there is nothing to dispatch, so wait up to 3 x $interval (heartbeat for hung workers).
wait_for_event() {
  local pids stamp waited=0 limit=$interval
  pids=$(worker_pids)
  stamp=$(results_stamp)
  [ "$(echo $pids | wc -w)" -ge "$cap" ] && limit=$((interval * 3))
  while [ "$waited" -lt "$limit" ]; do
    sleep "$poll"
    waited=$((waited + poll))
    [ -e .agent-runs/STOP ] && return
    if [ "$(results_stamp)" != "$stamp" ]; then echo "  worker result written, next pass now"; return; fi
    for pid in $pids; do
      kill -0 "$pid" 2>/dev/null || { echo "  worker $pid ended, next pass now"; return; }
    done
  done
}

while true; do
  ts=$(date +%Y%m%d-%H%M%S)
  echo "[$ts] pass start"
  attach=()
  if curl -s -o /dev/null -m 2 "$web"; then attach=(--attach "$web" --dir "$PWD"); echo "  on web UI $web"; fi
  OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run ${attach[@]+"${attach[@]}"} \
    --command autopilot \
    --format json \
    --title "autopilot $ts" \
    "single pass" < /dev/null > "$dir/$ts.jsonl" 2> "$dir/$ts.err"

  jq -rs '[.[] | select(.type=="text")] | last | .part.text // ""' "$dir/$ts.jsonl" > "$dir/$ts.md"
  jq -cs '[.[] | select(.type=="step_finish") | .part.tokens]
    | {input: (map(.input) | add), cached: (map(.cache.read) | add), output: (map(.output) | add)}' \
    "$dir/$ts.jsonl"
  head -3 "$dir/$ts.md"

  if head -1 "$dir/$ts.md" | grep -q '^AUTOPILOT: STOP'; then
    echo "[$ts] coordinator asked to stop"; break
  fi
  if grep -qiE 'usage limit|rate limit|quota|429|try again' "$dir/$ts.err" "$dir/$ts.jsonl" 2>/dev/null \
     && [ ! -s "$dir/$ts.md" ]; then
    echo "[$ts] usage limit, waiting 15 min"; sleep 900; continue
  fi
  [ -e .agent-runs/STOP ] && { echo "[$ts] STOP file found"; break; }
  wait_for_event
done
