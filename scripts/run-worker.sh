#!/usr/bin/env bash
# Runs one Codex worker on one wave, in its own git worktree.
#
#   scripts/run-worker.sh <wave> <branch> <brief-file>
#   scripts/run-worker.sh W0a feat/w0a-api briefs/W0a.md
#
# Writes, for the coordinator:
#   .agent-runs/<wave>.md     the worker's final message
#   .agent-runs/<wave>.jsonl  every event; token usage is in "turn.completed" events
# Profile `worker` = ~/.codex/worker.config.toml (cheap model, no prompts, network on).
set -euo pipefail

wave=$1 branch=$2 brief=$3
root=$(git rev-parse --show-toplevel)
wt="$root/../$(basename "$root")-wt/$branch"
mkdir -p "$root/.agent-runs"

git -C "$root" fetch origin main
if [ ! -d "$wt" ]; then
  git -C "$root" worktree add "$wt" -b "$branch" origin/main
fi

# Reuse the main checkout's web dependencies when they exist.
if [ -d "$root/web/node_modules" ] && [ ! -e "$wt/web/node_modules" ] && [ -d "$wt/web" ]; then
  ln -s "$root/web/node_modules" "$wt/web/node_modules"
fi

codex exec \
  --profile worker \
  --json \
  -C "$wt" \
  -o "$root/.agent-runs/$wave.md" \
  "$(cat "$brief")" < /dev/null > "$root/.agent-runs/$wave.jsonl"

# Token usage summary for STATUS.md.
jq -s '[.[] | select(.type=="turn.completed") | .usage]
  | {input: (map(.input_tokens) | add), cached_input: (map(.cached_input_tokens) | add),
     cache_write: (map(.cache_write_input_tokens // 0) | add), output: (map(.output_tokens) | add),
     reasoning: (map(.reasoning_output_tokens // 0) | add)}' "$root/.agent-runs/$wave.jsonl"
