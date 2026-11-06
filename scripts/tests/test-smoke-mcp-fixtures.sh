#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
source "$ROOT/scripts/tests/mcp-session-assert.sh"
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT

cat >"$fixture/prompt-only.jsonl" <<'JSON'
{"type":"text","part":{"text":"list_issues MCP seeded issue W15e mcp-seeded-session provider timeout"}}
JSON
if assert_mcp_tool_results "$fixture/prompt-only.jsonl"; then
  echo 'prompt-only fixture unexpectedly passed' >&2
  exit 1
fi

cat >"$fixture/failed.jsonl" <<'JSON'
{"type":"tool_result","name":"list_issues","isError":true,"result":"MCP seeded issue"}
{"type":"tool_result","name":"wave_status","status":"error","result":"W15e"}
{"type":"tool_result","name":"my_costs","state":{"status":"error"},"result":"mcp-seeded-session"}
{"type":"tool_result","name":"similar_failures","error":"provider timeout"}
JSON
if assert_mcp_tool_results "$fixture/failed.jsonl"; then
  echo 'failed-result fixture unexpectedly passed' >&2
  exit 1
fi

cat >"$fixture/restored.jsonl" <<'JSON'
{"type":"tool_result","name":"list_issues","isError":false,"result":{"title":"MCP seeded issue"}}
{"type":"tool_result","name":"wave_status","result":{"name":"W15e","state":"in_progress"}}
{"type":"tool_result","name":"my_costs","result":{"session_id":"mcp-seeded-session","output_tokens":5}}
{"type":"tool_result","name":"similar_failures","result":{"message":"The provider timeout was recorded."}}
JSON
assert_mcp_tool_results "$fixture/restored.jsonl"
echo 'MCP session fixtures passed: prompt-only and failed results rejected; restored tool results accepted'
