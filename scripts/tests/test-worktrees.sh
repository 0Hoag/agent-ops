#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
SCRIPT=${WORKTREES_SCRIPT:-$ROOT/scripts/worktrees.sh}
scratch=$(mktemp -d)
scratch=$(cd "$scratch" && pwd -P)
busy_pid=""
cleanup() {
  if [ -n "$busy_pid" ]; then
    kill "$busy_pid" 2>/dev/null || true
    wait "$busy_pid" 2>/dev/null || true
  fi
  rm -rf "$scratch"
}
trap cleanup EXIT
mkdir -p "$scratch/bin"
cat > "$scratch/bin/gh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$scratch/bin/gh"
export PATH="$scratch/bin:$PATH"

git init -q --bare "$scratch/origin.git"
git init -q "$scratch/repo"
git -C "$scratch/repo" config user.name 'Worktree test'
git -C "$scratch/repo" config user.email 'worktree-test@example.invalid'
printf '.agent-runs/\ntest-results/\n' > "$scratch/repo/.gitignore"
git -C "$scratch/repo" add .gitignore
git -C "$scratch/repo" commit -qm 'Initial fixture'
git -C "$scratch/repo" remote add origin "$scratch/origin.git"
git -C "$scratch/repo" push -q origin HEAD:refs/heads/main

fail() { echo "FAIL: $*" >&2; exit 1; }

run_script() {
  # Linux pgrep sees ancestor argv: do not put the target path on our own command line.
  TEST_MODE="$mode" TEST_WT="$wt" TEST_REPO="$scratch/repo" TEST_SCRIPT="$SCRIPT" bash -c '
    set -- "$TEST_MODE"
    [ "$TEST_MODE" != --remove ] || set -- "$@" "$TEST_WT"
    source "$TEST_SCRIPT" "$@" "$TEST_REPO"
  '
}

for mode in --prune --remove; do
  for evidence_dir in .agent-runs test-results; do
    wt="$scratch/evidence"
    git -C "$scratch/repo" worktree add -q --detach "$wt" HEAD
    mkdir -p "$wt/$evidence_dir/run2"
    evidence="$wt/$evidence_dir/run2/acceptance.txt"
    printf 'Unarchived acceptance evidence\n' > "$evidence"
    git -C "$wt" check-ignore -q "$evidence" || fail "fixture is not ignored: $evidence"
    output=$(run_script)
    [ -f "$evidence" ] || fail "$mode lost evidence: $evidence; output: $output"
    [ "$(cat "$evidence")" = 'Unarchived acceptance evidence' ] || fail "changed evidence: $evidence"
    [[ "$output" == *EVIDENCE* && "$output" == *'(kept)'* ]] || fail "missing evidence keep reason: $output"
    echo "PASS: $mode preserves $evidence_dir/run2/acceptance.txt with EVIDENCE keep reason"
    # Only disposable fixture content is removed; no project worktrees are touched.
    git -C "$scratch/repo" worktree remove --force "$wt"
  done

  wt="$scratch/clean"
  git -C "$scratch/repo" worktree add -q --detach "$wt" HEAD
  # A real process naming the worktree must still prevent both removal modes.
  mkfifo "$scratch/busy-input"
  bash -c 'echo ready > "$1"; read -r _ < "$2"' "$wt" "$scratch/busy-ready" "$scratch/busy-input" &
  busy_pid=$!
  for ((attempt=0; attempt<100; attempt++)); do
    [ ! -s "$scratch/busy-ready" ] || break
    sleep 0.1
  done
  [ -s "$scratch/busy-ready" ] || fail "busy fixture did not start"
  output=$(run_script)
  [ -d "$wt" ] || fail "$mode removed busy worktree: $output"
  [[ "$output" == *BUSY* && "$output" == *'(kept)'* ]] || fail "missing busy keep reason: $output"
  echo "PASS: $mode preserves genuinely busy evidence-free worktree"
  kill "$busy_pid"
  wait "$busy_pid" 2>/dev/null || true
  busy_pid=""
  rm "$scratch/busy-ready" "$scratch/busy-input"

  output=$(run_script)
  [ ! -d "$wt" ] || fail "$mode did not remove eligible clean worktree: $output"
  [[ "$output" == *'removed  '* ]] || fail "missing removal report: $output"
  echo "PASS: $mode removes eligible clean evidence-free worktree"
done
