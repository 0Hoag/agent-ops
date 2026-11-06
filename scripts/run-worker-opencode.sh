#!/usr/bin/env bash
# Runs one OpenCode worker on one wave, in its own git worktree.
#
#   scripts/run-worker-opencode.sh <wave> <branch> <brief-file>
#   scripts/run-worker-opencode.sh --target-repo <repo> --worktree <wt> <wave> <branch> <brief-file>
#   scripts/run-worker-opencode.sh W0a feat/w0a-api briefs/W0a.md
#
# Writes, for the coordinator:
#   .agent-runs/<wave>.md     the worker's final message
#   .agent-runs/<wave>.jsonl  every event; token usage is in "step_finish" events
# Agent `worker` = .opencode/agents/worker.md (cheap model, never asks, cannot merge).
set -euo pipefail

usage() { echo "usage: $0 [--target-repo <repo> --worktree <wt>] <wave> <branch> <brief-file>" >&2; exit 2; }
target_repo= worktree=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --target-repo) [[ $# -ge 2 ]] || usage; target_repo=$2; shift 2 ;;
    --worktree) [[ $# -ge 2 ]] || usage; worktree=$2; shift 2 ;;
    --*) echo "unknown option: $1" >&2; usage ;;
    *) break ;;
  esac
done
[[ $# -eq 3 ]] || usage
wave=$1 branch=$2 brief=$3
launcher_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && git rev-parse --show-toplevel)
. "$launcher_root/scripts/autopilot-state.sh"
autopilot_preflight opencode || exit 2
worker=opencode
model=${AGENT_RUN_MODEL:-openai/gpt-6-luna}
if [[ -n $target_repo || -n $worktree ]]; then
  [[ -n $target_repo && -n $worktree ]] || { echo 'target-repo and worktree must be provided together' >&2; exit 2; }
  target_repo=$(cd "$target_repo" && pwd -P)
  wt=$(cd "$worktree" && pwd -P)
else
  target_repo=$launcher_root
  wt="$target_repo/../$(basename "$target_repo")-wt/$branch"
fi
if [[ ! -d $wt/.git && ! -f $wt/.git ]]; then
  if [[ -n $worktree ]]; then echo "explicit worktree does not exist: $wt" >&2; exit 2; fi
  git -C "$target_repo" fetch origin reviews/development
  git -C "$target_repo" worktree add "$wt" -b "$branch" origin/reviews/development
fi
remote=$(git -C "$wt" remote get-url origin 2>/dev/null) || { echo "worktree has no origin remote: $wt" >&2; exit 2; }
current_branch=$(git -C "$wt" symbolic-ref --short HEAD 2>/dev/null) || { echo "worktree HEAD is detached: $wt" >&2; exit 2; }
git -C "$wt" rev-parse --verify HEAD >/dev/null 2>&1 || { echo "worktree HEAD is invalid: $wt" >&2; exit 2; }
[[ $current_branch == "$branch" ]] || { echo "worktree branch mismatch: expected $branch, got $current_branch" >&2; exit 2; }
wt_common=$(cd "$wt" && cd "$(git rev-parse --git-common-dir)" && pwd -P)
repo_common=$(cd "$target_repo" && cd "$(git rev-parse --git-common-dir)" && pwd -P)
[[ $wt_common == "$repo_common" ]] || { echo "worktree is not in target repository: $wt" >&2; exit 2; }
[[ -f $brief ]] || { echo "brief file not found: $brief" >&2; exit 2; }
mkdir -p "$target_repo/.agent-runs"

# Skip ~/.claude and ~/.agents skills: the GitHub/Codex orchestrator-workers skill comes from opencode.json.
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
set +e
# opencode v2 has no --dir: run from the worktree.
( cd "$wt" && OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run \
  --agent worker \
  --format json \
  --title "$wave" \
  "$(cat "$brief")" < /dev/null > "$target_repo/.agent-runs/$wave.jsonl" )
worker_status=$?
set -e
ended_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)

jq -rs '[.[] | select(.type=="text")] | last | .part.text // ""' \
  "$target_repo/.agent-runs/$wave.jsonl" > "$target_repo/.agent-runs/$wave.md"

# Token usage summary for STATUS.md.
usage=$(jq -sc '[.[] | select(.type=="step_finish") | .part.tokens] as $t
  | {input: ([$t[].input // 0] | add), cached_input: ([$t[].cache.read // 0] | add),
     cache_write: ([$t[].cache.write // 0] | add), output: ([$t[].output // 0] | add),
     reasoning: ([$t[].reasoning // 0] | add)}' "$target_repo/.agent-runs/$wave.jsonl")
printf '%s\n' "$usage"
printf '%s\n' "$usage" > "$target_repo/.agent-runs/$wave-usage.json"
source "$launcher_root/scripts/report-agent-run.sh"
report_agent_run "$target_repo/.agent-runs/$wave.jsonl" opencode || printf '%s\n' 'Agent run API reporting failed' >&2
exit "$worker_status"
