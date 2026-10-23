#!/usr/bin/env bash
# Coordinator loop: the shell waits, the model only works.
# Each pass is a fresh, short OpenCode session (`/autopilot` = one pass of AGENTS.md → Autopilot), then
# the shell sleeps. Workers run detached (scripts/spawn.sh) and outlive the pass.
#
#   scripts/autopilot.sh                 # pass every 10 min until stopped
#   INTERVAL=300 scripts/autopilot.sh    # every 5 min
#   touch .agent-runs/STOP               # stop after the current pass
#
# Per pass: .agent-runs/coordinator/<time>.jsonl (events) and .md (final message).
# A final message whose first line starts with "AUTOPILOT: STOP" ends the loop.
set -uo pipefail

cd "$(git rev-parse --show-toplevel)"
interval=${INTERVAL:-600}
dir=.agent-runs/coordinator
mkdir -p "$dir"
rm -f .agent-runs/STOP

while true; do
  ts=$(date +%Y%m%d-%H%M%S)
  echo "[$ts] pass start"
  OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run \
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
  sleep "$interval"
done
