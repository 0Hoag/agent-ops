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
worker=codex
model=${AGENT_RUN_MODEL:-gpt-reserve}
root=$(git rev-parse --show-toplevel)
wt="$root/../$(basename "$root")-wt/$branch"
git_common_dir=$(git -C "$root" rev-parse --path-format=absolute --git-common-dir)
mkdir -p "$root/.agent-runs"

git -C "$root" fetch origin main
if [ ! -d "$wt" ]; then
  git -C "$root" worktree add "$wt" -b "$branch" origin/main
fi
# Common objects/refs and this worktree's index/HEAD live outside the worktree.
# Codex protects the linked Git directory separately, so grant both explicitly.
git_worktree_dir=$(git -C "$wt" rev-parse --absolute-git-dir)

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
set +e
codex exec \
  --profile worker \
  --json \
  -C "$wt" \
  --add-dir "$git_common_dir" \
  --add-dir "$git_worktree_dir" \
  -o "$root/.agent-runs/$wave.md" \
  "$(cat "$brief")" < /dev/null > "$root/.agent-runs/$wave.jsonl"
worker_status=$?
set -e
[[ -f "$root/.agent-runs/$wave.md" ]] || : > "$root/.agent-runs/$wave.md"
ended_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Token usage summary for STATUS.md.
usage=$(jq -sc '([.[] | select(.type=="turn.completed") | .usage] as $u
  | {input: ([$u[].input_tokens // 0] | add), cached_input: ([$u[].cached_input_tokens // 0] | add),
     cache_write: ([$u[].cache_write_input_tokens // 0] | add), output: ([$u[].output_tokens // 0] | add),
     reasoning: ([$u[].reasoning_output_tokens // 0] | add)})' "$root/.agent-runs/$wave.jsonl")
printf '%s\n' "$usage"
printf '%s\n' "$usage" > "$root/.agent-runs/$wave-usage.json"
source "$root/scripts/report-agent-run.sh"
report_agent_run "$usage" "$started_at" "$ended_at" || printf '%s\n' 'Agent run API reporting failed' >&2
exit "$worker_status"
