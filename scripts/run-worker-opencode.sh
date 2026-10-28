#!/usr/bin/env bash
# Runs one OpenCode worker on one wave, in its own git worktree.
#
#   scripts/run-worker-opencode.sh <wave> <branch> <brief-file>
#   scripts/run-worker-opencode.sh W0a feat/w0a-api briefs/W0a.md
#
# Writes, for the coordinator:
#   .agent-runs/<wave>.md     the worker's final message
#   .agent-runs/<wave>.jsonl  every event; token usage is in "step_finish" events
# Agent `worker` = .opencode/agents/worker.md (cheap model, never asks, cannot merge).
set -euo pipefail

wave=$1 branch=$2 brief=$3
worker=opencode
model=${AGENT_RUN_MODEL:-openai/gpt-6-luna}
root=$(git rev-parse --show-toplevel)
wt="$root/../$(basename "$root")-wt/$branch"
mkdir -p "$root/.agent-runs"

git -C "$root" fetch origin main
if [ ! -d "$wt" ]; then
  git -C "$root" worktree add "$wt" -b "$branch" origin/main
fi

# Skip ~/.claude and ~/.agents skills: the GitHub/Codex orchestrator-workers skill comes from opencode.json.
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
set +e
OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run \
  --agent worker \
  --dir "$wt" \
  --format json \
  --title "$wave" \
  "$(cat "$brief")" < /dev/null > "$root/.agent-runs/$wave.jsonl"
worker_status=$?
set -e
ended_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)

jq -rs '[.[] | select(.type=="text")] | last | .part.text // ""' \
  "$root/.agent-runs/$wave.jsonl" > "$root/.agent-runs/$wave.md"

# Token usage summary for STATUS.md.
usage=$(jq -sc '[.[] | select(.type=="step_finish") | .part.tokens] as $t
  | {input: ([$t[].input // 0] | add), cached_input: ([$t[].cache.read // 0] | add),
     cache_write: ([$t[].cache.write // 0] | add), output: ([$t[].output // 0] | add),
     reasoning: ([$t[].reasoning // 0] | add)}' "$root/.agent-runs/$wave.jsonl")
printf '%s\n' "$usage"
printf '%s\n' "$usage" > "$root/.agent-runs/$wave-usage.json"
source "$root/scripts/report-agent-run.sh"
report_agent_run "$root/.agent-runs/$wave.jsonl" opencode || printf '%s\n' 'Agent run API reporting failed' >&2
exit "$worker_status"
