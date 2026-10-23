#!/usr/bin/env bash
# Starts a command fully detached (new session, no controlling terminal) so it survives the
# coordinator pass that started it. Output goes to <log>.
#
#   scripts/spawn.sh <log> <command> [args...]
#   scripts/spawn.sh .agent-runs/W1a.log scripts/run-worker.sh W1a feat/w1a-x briefs/W1a.md
set -euo pipefail

log=$1
shift
mkdir -p "$(dirname "$log")"
python3 -c 'import os, sys
os.setsid()
fd = os.open(sys.argv[1], os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o644)
os.dup2(fd, 1); os.dup2(fd, 2)
os.dup2(os.open(os.devnull, os.O_RDONLY), 0)
os.execvp(sys.argv[2], sys.argv[2:])' "$log" "$@" &
echo "spawned pid $! -> $log"
