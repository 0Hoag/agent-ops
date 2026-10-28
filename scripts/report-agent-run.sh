#!/usr/bin/env bash
# Best-effort API reporting; credentials are only sent in the request header.
set -euo pipefail

report_agent_run() {
  local transcript=$1 harness=$2
  [[ -n ${AGENT_RUN_API_URL:-} && -n ${AGENT_RUN_TOKEN:-} && -n ${AGENT_RUN_REPOSITORY_ID:-} ]] || return 0
  local endpoint=${AGENT_RUN_API_URL%/}
  [[ $endpoint == */api/v1/agent-runs ]] || endpoint="$endpoint/api/v1/agent-runs"
  local cli=${AGENTOPS_CLI:-$(git -C "$root" rev-parse --show-toplevel)/api/bin/agentops}
  [[ -x $cli ]] || cli=agentops
  AGENT_RUN_MODEL=$model AGENT_RUN_ISSUES=${AGENT_RUN_ISSUES:-[]} "$cli" push --harness "$harness" --model "$model" --endpoint "$endpoint" \
    --repository-id "$AGENT_RUN_REPOSITORY_ID" --wave "$wave" --worker "$worker" --issue-numbers "${AGENT_RUN_ISSUES:-[]}" "$transcript"
}
