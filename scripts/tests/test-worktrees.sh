#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
SCRIPT=${WORKTREES_SCRIPT:-$ROOT/scripts/worktrees.sh}
scratch=$(mktemp -d)
scratch=$(cd "$scratch" && pwd -P)
trap 'rm -rf "$scratch"' EXIT
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

for mode in --prune --remove; do
  for evidence_dir in .agent-runs test-results; do
    wt="$scratch/evidence"
    git -C "$scratch/repo" worktree add -q --detach "$wt" HEAD
    mkdir -p "$wt/$evidence_dir/run2"
    evidence="$wt/$evidence_dir/run2/acceptance.txt"
    printf 'Unarchived acceptance evidence\n' > "$evidence"
    git -C "$wt" check-ignore -q "$evidence" || fail "fixture is not ignored: $evidence"
    args=("$mode")
    [ "$mode" != --remove ] || args+=("$wt")
    output=$(bash "$SCRIPT" "${args[@]}" "$scratch/repo")
    [ -f "$evidence" ] || fail "$mode lost evidence: $evidence; output: $output"
    [ "$(cat "$evidence")" = 'Unarchived acceptance evidence' ] || fail "changed evidence: $evidence"
    [[ "$output" == *EVIDENCE* && "$output" == *'(kept)'* ]] || fail "missing evidence keep reason: $output"
    echo "PASS: $mode preserves $evidence_dir/run2/acceptance.txt with EVIDENCE keep reason"
    # Only disposable fixture content is removed; no project worktrees are touched.
    git -C "$scratch/repo" worktree remove --force "$wt"
  done

  wt="$scratch/clean"
  git -C "$scratch/repo" worktree add -q --detach "$wt" HEAD
  args=("$mode")
  [ "$mode" != --remove ] || args+=("$wt")
  output=$(bash "$SCRIPT" "${args[@]}" "$scratch/repo")
  [ ! -d "$wt" ] || fail "$mode did not remove eligible clean worktree: $output"
  [[ "$output" == *'removed  '* ]] || fail "missing removal report: $output"
  echo "PASS: $mode removes eligible clean evidence-free worktree"
done
