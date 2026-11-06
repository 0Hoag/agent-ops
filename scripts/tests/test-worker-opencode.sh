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
printf '%s\n' "$*" >> "$OPENCODE_PROOF"
printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":3,"output":2,"reasoning":1,"cache":{"read":4,"write":5}}}}'
printf '%s\n' '{"type":"text","part":{"text":"fixture"}}'
EOF
chmod +x "$tmp/harness/bin/opencode"
git -C "$tmp/client" worktree add -q "$tmp/client-wt" -b client-worker
proof=$tmp/proof

# The old launcher derives helpers from the caller and fails before OpenCode for a client CWD.
git show HEAD:scripts/run-worker-opencode.sh > "$tmp/old-launcher"
if (cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" bash "$tmp/old-launcher" W17q client-worker "$tmp/brief") 2>"$tmp/old-error"; then exit 1; fi
grep -Eq 'autopilot-state|No such file' "$tmp/old-error"

(cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" \
  bash "$tmp/harness/scripts/run-worker-opencode.sh" --target-repo "$tmp/client" \
  --worktree "$tmp/client-wt" W17q client-worker "$tmp/brief")
[[ $(sed -n '1p' "$proof") == "$tmp/client-wt" ]]
! grep -q -- '--dir' "$proof"
grep -q -- '--agent worker --format json --title W17q' "$proof"
[[ -f "$tmp/client/.agent-runs/W17q.jsonl" && -f "$tmp/client/.agent-runs/W17q-usage.json" ]]
[[ $(jq -r '.input' "$tmp/client/.agent-runs/W17q-usage.json") == 3 ]]
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
[[ -f "$tmp/harness/.agent-runs/W17d-usage.json" ]]

# Identical-input negative control: remove only the branch guard; the mismatched input must then run.
cp "$tmp/harness/scripts/run-worker-opencode.sh" "$tmp/harness/scripts/control-launcher"
sed -i.bak '/\[\[ \$current_branch == "\$branch" \]\]/d' "$tmp/harness/scripts/control-launcher"
if (cd "$tmp/client-wt" && PATH="$tmp/harness/bin:$PATH" OPENCODE_PROOF="$proof" \
  bash "$tmp/harness/scripts/control-launcher" --target-repo "$tmp/client" --worktree "$tmp/client-wt" \
  W17q wrong-branch "$tmp/brief"); then :; else exit 1; fi
echo 'PASS worker OpenCode explicit target and validation'
