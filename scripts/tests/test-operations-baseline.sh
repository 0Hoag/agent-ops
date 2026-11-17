#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
repo="$tmp/repo"; mkdir "$repo"; git -C "$repo" init -q
git -C "$repo" config user.email test@example.invalid; git -C "$repo" config user.name test
commit() { printf '%s\n' "$1" >"$repo/file"; git -C "$repo" add file; git -C "$repo" commit -qm "$1"; }
commit base; base=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" branch development
git -C "$repo" checkout -q development; commit development; development=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" checkout -q main; git -C "$repo" merge --no-ff -qm promotion development; promotion=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" update-ref refs/remotes/origin/main "$promotion"
git -C "$repo" update-ref refs/remotes/origin/reviews/development "$development"

resolved=$(OPERATIONS_MAIN_REF=origin/main OPERATIONS_DEVELOPMENT_REF=origin/reviews/development bash "$root/scripts/verify/resolve-previous-release.sh" "$repo")
test "$resolved" = "$base"
override=$(PREVIOUS_RELEASE_REF="$development" OPERATIONS_MAIN_REF=origin/main OPERATIONS_DEVELOPMENT_REF=origin/reviews/development bash "$root/scripts/verify/resolve-previous-release.sh" "$repo")
test "$override" = "$development"

missing="$tmp/missing"; mkdir "$missing"; git -C "$missing" init -q
if OPERATIONS_MAIN_REF=origin/main OPERATIONS_DEVELOPMENT_REF=origin/reviews/development bash "$root/scripts/verify/resolve-previous-release.sh" "$missing" >"$tmp/missing.out" 2>&1; then
  echo 'CONTROL FAIL: missing baseline unexpectedly passed' >&2; exit 1
fi
grep -Fq 'no development-to-main promotion baseline could be resolved' "$tmp/missing.out"

mutated="$tmp/mutated.sh"
sed 's/if \[\[ -n ${PREVIOUS_RELEASE_REF:-} \]\]; then/if false; then/' "$root/scripts/verify/resolve-previous-release.sh" > "$mutated"
mutated_result=$(OPERATIONS_MAIN_REF=origin/main PREVIOUS_RELEASE_REF="$development" bash "$mutated" "$repo")
if test "$mutated_result" = "$development"; then
  echo 'CONTROL FAIL: explicit override mutation preserved override precedence' >&2; exit 1
fi
printf 'PASS promotion parent=%s explicit override=%s missing-baseline=blocked\n' "$resolved" "$override"
echo 'CONTROL PASS: identical override input fails when precedence guard is mutated'
