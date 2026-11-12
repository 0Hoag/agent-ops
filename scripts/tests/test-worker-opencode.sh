#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git_init() {
  git -C "$1" init -q
  git -C "$1" config user.email fixture@example.invalid
  git -C "$1" config user.name Fixture
  git -C "$1" commit --allow-empty -qm fixture
  git -C "$1" branch -M reviews/development
  git -C "$1" remote add origin "$1"
}
mkdir -p "$tmp/harness/scripts" "$tmp/harness/bin" "$tmp/client" "$tmp/client-wt"
git_init "$tmp/harness"
git_init "$tmp/client"
cp "$root/scripts/run-worker-opencode.sh" "$tmp/harness/scripts/"
cp "$root/scripts/autopilot-state.sh" "$tmp/harness/scripts/"
cp "$root/scripts/report-agent-run.sh" "$tmp/harness/scripts/"
printf 'fixture brief\n' > "$tmp/brief"
cat > "$tmp/harness/bin/opencode" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$PWD" > "$OPENCODE_PROOF"
printf '%s\n' "$@" > "$OPENCODE_ARGV"
printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":3,"output":2,"reasoning":1,"cache":{"read":4,"write":5}}}}'
printf '%s\n' '{"type":"text","part":{"text":"fixture"}}'
EOF
chmod +x "$tmp/harness/bin/opencode"
cat > "$tmp/harness/bin/agentops" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$AGENT_RUN_MODEL" > "$REPORT_PROOF"
while [[ $# -gt 0 ]]; do
  if [[ $1 == --model ]]; then printf '%s\n' "$2" >> "$REPORT_PROOF"; fi
  shift
done
EOF
chmod +x "$tmp/harness/bin/agentops"
unset AGENT_RUN_MODEL
export AGENT_RUN_API_URL=https://fixture.invalid AGENT_RUN_TOKEN=fixture AGENT_RUN_REPOSITORY_ID=fixture
export AGENTOPS_CLI="$tmp/harness/bin/agentops" REPORT_PROOF="$tmp/report-proof" OPENCODE_ARGV="$tmp/argv"
assert_model() {
  local expected=$1 title=$2
  printf '%s\n' run --model "$expected" --agent worker --format json --title "$title" 'fixture brief' > "$tmp/expected-argv"
  if ! diff -u "$tmp/expected-argv" "$OPENCODE_ARGV"; then
    echo "FAIL worker argv: missing/wrong model (expected --model $expected)" >&2
    exit 1
  fi
  printf '%s\n' "$expected" "$expected" > "$tmp/expected-report"
  if ! diff -u "$tmp/expected-report" "$REPORT_PROOF"; then
    echo "FAIL reported model differs from invoked selection $expected" >&2
    exit 1
  fi
  echo "PASS exact argv and reporting model: $expected"
}
git -C "$tmp/client" worktree add -q "$tmp/client-wt" -b client-worker
proof=$tmp/proof

(cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" \
  bash "$tmp/harness/scripts/run-worker-opencode.sh" --target-repo "$tmp/client" \
  --worktree "$tmp/client-wt" W17q client-worker "$tmp/brief")
[[ $(sed -n '1p' "$proof") == "$tmp/client-wt" ]]
assert_model openai/gpt-6-luna W17q
[[ -f "$tmp/client/.agent-runs/W17q.jsonl" && -f "$tmp/client/.agent-runs/W17q-usage.json" ]]
[[ $(jq -r '.input' "$tmp/client/.agent-runs/W17q-usage.json") == 3 ]]
(cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" AGENT_RUN_MODEL=openai/fixture-override \
  bash "$tmp/harness/scripts/run-worker-opencode.sh" --target-repo "$tmp/client" \
  --worktree "$tmp/client-wt" W17q client-worker "$tmp/brief")
assert_model openai/fixture-override W17q
if (cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" \
  bash "$tmp/harness/scripts/run-worker-opencode.sh" --target-repo "$tmp/client" \
  --worktree "$tmp/client-wt" W17q wrong-branch "$tmp/brief") 2>"$tmp/mismatch"; then exit 1; fi
grep -q 'branch mismatch' "$tmp/mismatch"
if (cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" \
  bash "$tmp/harness/scripts/run-worker-opencode.sh" --target-repo "$tmp/harness" \
  --worktree "$tmp/client-wt" W17q client-worker "$tmp/brief") 2>"$tmp/repo-mismatch"; then exit 1; fi
grep -q 'not in target repository' "$tmp/repo-mismatch"

# Default backend invocation remains three positional arguments and creates its normal worktree.
mkdir -p "$tmp/harness-wt"
(cd "$tmp/harness" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" \
  bash scripts/run-worker-opencode.sh W17d default-worker "$tmp/brief")
[[ $(sed -n '1p' "$proof") == "$tmp/harness-wt/default-worker" ]]
assert_model openai/gpt-6-luna W17d
[[ -f "$tmp/harness/.agent-runs/W17d-usage.json" ]]

# Identical-input negative control: remove only the branch guard; the mismatched input must then run.
cp "$tmp/harness/scripts/run-worker-opencode.sh" "$tmp/harness/scripts/control-launcher"
sed -i.bak '/\[\[ \$current_branch == "\$branch" \]\]/d' "$tmp/harness/scripts/control-launcher"
if (cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" \
  bash "$tmp/harness/scripts/control-launcher" --target-repo "$tmp/client" --worktree "$tmp/client-wt" \
  W17q wrong-branch "$tmp/brief"); then :; else exit 1; fi
echo 'PASS worker OpenCode explicit target and validation'
