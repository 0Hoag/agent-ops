#!/usr/bin/env bash
set -uo pipefail
command -v docker >/dev/null 2>&1 || exit 0
docker builder prune -f --filter until=24h >/dev/null 2>&1 || true
projects=$(docker ps -a --filter status=exited --filter label=com.docker.compose.project --format '{{.ID}} {{.Label "com.docker.compose.project"}}' 2>/dev/null) || exit 0
while read -r id project; do
  [ -n "$id" ] || continue
  case "$project" in
    w97*|agent-run-*|agent-watch-*|agent-ops-smoke-*) docker rm "$id" >/dev/null 2>&1 || true ;;
  esac
done <<< "$projects"
