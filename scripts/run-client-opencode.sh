#!/usr/bin/env bash
# Run OpenCode in an existing client checkout/worktree; never create/reset/clean it.
# Usage: run-client-opencode.sh <wave> <worktree> <brief-file>
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <wave> <worktree> <brief-file>" >&2
  exit 2
fi
wave=$1
wt=$(cd "$2" && pwd -P)
brief=$(cd "$(dirname "$3")" && pwd -P)/$(basename "$3")
root=$(git rev-parse --show-toplevel)
if [[ ! -f "$brief" ]]; then
  echo "brief file not found: $brief" >&2
  exit 2
fi
repo=$(git -C "$wt" remote get-url origin)
case "$repo" in *agent-ops-client.git|*agent-ops-client) ;; *) echo "not an agent-ops-client remote: $repo" >&2; exit 2;; esac
mkdir -p "$root/.agent-runs"
run=$root/.agent-runs/$wave

prompt=$(printf 'WORKTREE: %s\nREMOTE: %s\n\n' "$wt" "$repo")
prompt+=$(<"$brief")
OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run --agent worker --dir "$wt" --format json --title "$wave" \
  "$prompt" < /dev/null > "$run.jsonl"
jq -rs '[.[] | select(.type=="text")] | last | .part.text // ""' "$run.jsonl" > "$run.md"
jq -s '[.[] | select(.type=="step_finish") | .part.tokens] | {input:(map(.input)|add // 0), cached_input:(map(.cache.read)|add // 0), cache_write:(map(.cache.write)|add // 0), output:(map(.output)|add // 0), reasoning:(map(.reasoning)|add // 0)}' "$run.jsonl" > "$run.tokens.json"
cat "$run.tokens.json"
