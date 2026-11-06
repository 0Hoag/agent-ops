#!/usr/bin/env bash
set -Eeuo pipefail

# Validate OpenCode's JSON event stream, not the prompt or assistant prose.
assert_mcp_tool_results() {
  local session=$1
  jq -e -s '
    def name: (.name // .tool // .tool_name // .function?.name // .part?.tool // .part?.name);
    def successful:
      ((.isError // .is_error // false) | not)
      and ((.error // null) == null)
      and ((.state?.status // "completed") != "error")
      and ((.status // "completed") != "error");
    def payload: tostring;
    [ .[] | .. | objects
      | select((.type == "tool_result" or .type == "tool-result" or .type == "tool")
          and (name != null) and successful)
      | {name: name, payload: payload}
    ] as $results
    | [
        {name: "list_issues", marker: "MCP seeded issue"},
        {name: "wave_status", marker: "W15e"},
        {name: "my_costs", marker: "mcp-seeded-session"},
        {name: "similar_failures", marker: "provider timeout"}
      ]
    | all(.[];
        . as $expected
        | any($results[]; .name == $expected.name and (.payload | test($expected.marker; "i"))))
  ' "$session" >/dev/null
}
