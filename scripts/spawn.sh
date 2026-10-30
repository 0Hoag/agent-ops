#!/usr/bin/env bash
# Starts a command fully detached (new session, no controlling terminal) so it survives the
# coordinator pass that started it. Output goes to <log>.
#
#   scripts/spawn.sh <log> <command> [args...]
#   scripts/spawn.sh .agent-runs/W1a.log scripts/run-worker.sh W1a feat/w1a-x briefs/W1a.md
set -euo pipefail

log=$1
shift
# Run budget: a wave runs at most MAX_RUN times (<wave>, <wave>-r2). A -r3 is refused: split the remainder into a new issue.
for a in "$@"; do
  if [[ $a =~ -r([0-9]+)$ ]] && [ "${BASH_REMATCH[1]}" -gt "${MAX_RUN:-2}" ]; then
    echo "refused: $a exceeds run budget ${MAX_RUN:-2}; open a follow-up issue for what is left" >&2; exit 1
  fi
done
mkdir -p "$(dirname "$log")"
python3 -c 'import os, sys
os.setsid()
fd = os.open(sys.argv[1], os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o644)
os.dup2(fd, 1); os.dup2(fd, 2)
os.dup2(os.open(os.devnull, os.O_RDONLY), 0)
os.execvp(sys.argv[2], sys.argv[2:])' "$log" "$@" &
echo "spawned pid $! -> $log"
