#!/usr/bin/env bash
# Starts a command fully detached (new session, no controlling terminal) so it survives the
# coordinator pass that started it. Output goes to <log>.
#
#   scripts/spawn.sh <log> <command> [args...]
#   scripts/spawn.sh .agent-runs/W1a.log scripts/run-worker.sh W1a feat/w1a-x briefs/W1a.md
set -euo pipefail

# Optional guarded form: spawn.sh --recovery <repo> <issue> <unit> <log> <command> [args...]
recovery=0
if [ "${1:-}" = --recovery ]; then
  [ "$#" -ge 6 ] || { echo 'usage: spawn.sh --recovery <repo> <issue> <unit> <log> <command> [args...]' >&2; exit 2; }
  script_dir=$(cd "$(dirname "$0")" && pwd)
  . "$script_dir/autopilot-state.sh"
  repo=$2 issue=$3 unit=$4 log=$5
  shift 5
  # Resolve command availability before consuming the one-shot reservation.
  command -v "$1" >/dev/null 2>&1 || { echo "preflight failed: missing launcher $1" >&2; exit 2; }
  case "$1" in
    */run-worker.sh) autopilot_preflight python3 codex || exit 2 ;;
    */run-worker-opencode.sh) autopilot_preflight python3 opencode || exit 2 ;;
    *) autopilot_preflight python3 || exit 2 ;;
  esac
  recovery=1
else
  [ "$#" -ge 2 ] || { echo 'usage: spawn.sh <log> <command> [args...]' >&2; exit 2; }
  log=$1; shift
fi
# Run budget: a wave runs at most MAX_RUN times (<wave>, <wave>-r2). A -r3 is refused: split the remainder into a new issue.
for a in "$@"; do
  if [[ $a =~ -r([0-9]+)$ ]] && [ "${BASH_REMATCH[1]}" -gt "${MAX_RUN:-2}" ]; then
    echo "refused: $a exceeds run budget ${MAX_RUN:-2}; open a follow-up issue for what is left" >&2; exit 1
  fi
done
[ "$recovery" -eq 0 ] || autopilot_reserve_recovery "$repo" "$issue" "$unit"
mkdir -p "$(dirname "$log")"
python3 -c 'import os, sys
os.setsid()
fd = os.open(sys.argv[1], os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o644)
os.dup2(fd, 1); os.dup2(fd, 2)
os.dup2(os.open(os.devnull, os.O_RDONLY), 0)
os.execvp(sys.argv[2], sys.argv[2:])' "$log" "$@" &
echo "spawned pid $! -> $log"
