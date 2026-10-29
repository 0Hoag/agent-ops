#!/usr/bin/env bash
set -euo pipefail
root=$(git rev-parse --show-toplevel)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/release-fixture.XXXXXX")
cleanup() { rm -rf "$fixture"; }
trap cleanup EXIT HUP INT TERM

git -C "$fixture" init -q
git -C "$fixture" config user.name tester
git -C "$fixture" config user.email tester@example.test
git -C "$fixture" commit --allow-empty -qm 'chore: fixture baseline'
run() { git -C "$fixture" "$@"; }
commit() { printf '%s\n' "$2" > "$fixture/change.txt"; run add change.txt; run commit -qm "$1" ${2:+'-m'} "${2:-}"; }
release() { (cd "$fixture" && bash "$root/scripts/release.sh" --dry-run); }
expect() { local actual=$1 expected=$2; [[ "$actual" == *"Proposed version: $expected"* ]] || { echo "expected $expected, got:\n$actual" >&2; exit 1; }; }

commit 'fix: correct parser' 'patch change'; expect "$(release)" 0.1.1
commit 'feat: add release notes' 'feature change'; expect "$(release)" 0.2.0
commit 'feat!: change output contract' 'breaking change'; expect "$(release)" 1.0.0
run tag v2.4.0
commit 'docs: explain release' 'not releasable'; expect "$(release)" 2.4.0
before=$(run rev-parse HEAD); index=$(run diff --cached); worktree=$(run status --porcelain); release >/dev/null
[[ "$before" == "$(run rev-parse HEAD)" && "$index" == "$(run diff --cached)" && "$worktree" == "$(run status --porcelain)" ]]
run tag v3.1.0
commit 'fix: tagged patch' 'tagged patch'; expect "$(release)" 3.1.1
run checkout -qb feature
commit 'feat: branch feature' 'branch feature'
run checkout -q master 2>/dev/null || run checkout -q main
run merge --no-ff -qm 'Merge branch feature' feature
merge_output=$(release); expect "$merge_output" 3.2.0
[[ "$merge_output" != *'Merge branch feature'* ]]

mutated=$(mktemp -d "${TMPDIR:-/tmp}/release-mutated.XXXXXX")
trap 'rm -rf "$fixture" "$mutated"' EXIT HUP INT TERM
cp "$root/scripts/release.sh" "$mutated/release.sh"
sed -i.bak 's/feat) minor=1/feat) patch=1/' "$mutated/release.sh"
if (cd "$fixture" && bash "$mutated/release.sh" --dry-run | grep -q 'Proposed version: 3.2.0'); then
  echo 'mutation control unexpectedly passed' >&2; exit 1
fi
echo 'release fixture tests passed (patch, feature, breaking, no-release, tags, merge exclusion, dry-run, mutation control)'
