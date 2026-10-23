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
root=$(git rev-parse --show-toplevel)
wt="$root/../$(basename "$root")-wt/$branch"
mkdir -p "$root/.agent-runs"

git -C "$root" fetch origin main
if [ ! -d "$wt" ]; then
  git -C "$root" worktree add "$wt" -b "$branch" origin/main
fi

if [ -d "$root/web/node_modules" ] && [ ! -e "$wt/web/node_modules" ] && [ -d "$wt/web" ]; then
  ln -s "$root/web/node_modules" "$wt/web/node_modules"
fi

# Skip ~/.claude and ~/.agents skills: the GitHub/Codex orchestrator-workers skill comes from opencode.json.
OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run \
  --agent worker \
  --dir "$wt" \
  --format json \
  --title "$wave" \
  "$(cat "$brief")" < /dev/null > "$root/.agent-runs/$wave.jsonl"

jq -rs '[.[] | select(.type=="text")] | last | .part.text // ""' \
  "$root/.agent-runs/$wave.jsonl" > "$root/.agent-runs/$wave.md"

# Token usage summary for STATUS.md.
jq -s '[.[] | select(.type=="step_finish") | .part.tokens]
  | {input: (map(.input) | add), cached_input: (map(.cache.read) | add),
     cache_write: (map(.cache.write) | add), output: (map(.output) | add),
     reasoning: (map(.reasoning) | add)}' "$root/.agent-runs/$wave.jsonl"
