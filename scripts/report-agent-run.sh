#!/usr/bin/env bash
# Best-effort API reporting; credentials are only sent in the request header.
set -euo pipefail

report_agent_run() {
  local usage=$1 started=$2 ended=$3
  [[ -n ${AGENT_RUN_API_URL:-} && -n ${AGENT_RUN_TOKEN:-} && -n ${AGENT_RUN_REPOSITORY_ID:-} ]] || return 0
  local payload
  payload=$(jq -cn --arg repo "$AGENT_RUN_REPOSITORY_ID" --arg wave "$wave" \
    --arg worker "$worker" --arg model "$model" --argjson issues "${AGENT_RUN_ISSUES:-[]}" \
    --arg started "$started" --arg ended "$ended" --argjson u "$usage" \
    '{repository_id:$repo,wave:$wave,worker:$worker,model:$model,issue_numbers:$issues,started_at:$started,ended_at:$ended,input_tokens:$u.input,cached_input_tokens:$u.cached_input,cache_write_tokens:$u.cache_write,output_tokens:$u.output,reasoning_tokens:$u.reasoning}')
  curl --silent --show-error --fail -X POST "${AGENT_RUN_API_URL%/}/api/v1/agent-runs" \
    -H 'Content-Type: application/json' -H "X-Agent-Run-Token: $AGENT_RUN_TOKEN" -d "$payload" >/dev/null
}
