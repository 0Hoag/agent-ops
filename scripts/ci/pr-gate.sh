#!/usr/bin/env bash
# Per-PR gate, run in CI on pull_request (checkout with fetch-depth 0). Fails when a PR that changes product code
#   1. has no `Closes #n` in its body,
#   2. changes product code without changing any test, or
#   3. has tests that still pass once the product change is reverted ("fails before, passes after").
# Only feat/fix PRs are checked; test/ci/docs/chore/refactor PRs pass.
#
#   PR_TITLE=... PR_BODY=... scripts/ci/pr-gate.sh <base-sha> <go|vitest> [product-dir]
# go:     product = <product-dir>/**/*.go except *_test.go, docs/, migrations/, generated/
# vitest: product = <product-dir>/**/*.{ts,tsx} except tests, *.d.ts and generated/
# Step 3 rewrites the working tree: run it last.
set -euo pipefail

base=$1 mode=$2 dir=${3:-.}
title=${PR_TITLE:-}
type=${title%%[(:]*}
case "$type" in feat|fix) ;; *) echo "pr-gate: '$type' PR, skipped"; exit 0 ;; esac

changed() { git diff --name-only --diff-filter="$1" "$base"...HEAD -- "$dir"; }
if [ "$mode" = go ]; then
  is_test() { [[ $1 == *_test.go ]]; }
  is_product() { [[ $1 == *.go && $1 != *_test.go && $1 != */docs/* && $1 != */migrations/* && $1 != */generated/* ]]; }
else
  is_test() { [[ $1 == *.test.ts || $1 == *.test.tsx ]]; }
  is_product() { [[ ($1 == *.ts || $1 == *.tsx) && $1 != *.test.* && $1 != *.d.ts && $1 != */generated/* ]]; }
fi

product=() tests=()
while IFS= read -r f; do [ -n "$f" ] && is_product "$f" && product+=("$f"); done < <(changed AMD)
while IFS= read -r f; do [ -n "$f" ] && is_test "$f" && tests+=("$f"); done < <(changed AM)
[ ${#product[@]} -eq 0 ] && { echo "pr-gate: no product code changed, skipped"; exit 0; }

grep -Eqi '(closes|fixes|resolves) +(0Hoag/[a-z-]+)?#[0-9]+' <<<"${PR_BODY:-}" \
  || { echo "::error::PR body needs 'Closes #n'"; exit 1; }
[ ${#tests[@]} -gt 0 ] \
  || { echo "::error::feat/fix PR changes product code (${product[*]}) but no test"; exit 1; }

# Revert product files to base, keep the PR's tests.
for f in "${product[@]}"; do
  if git cat-file -e "$base:$f" 2>/dev/null; then git checkout "$base" -- "$f"; else rm -f "$f"; fi
done

if [ "$mode" = go ]; then
  pkgs=$(for t in "${tests[@]}"; do echo "./$(dirname "${t#"$dir"/}")"; done | sort -u)
  cd "$dir"
  # shellcheck disable=SC2086
  if go test -count=1 $pkgs >/tmp/fail-before.log 2>&1; then
    cat /tmp/fail-before.log
    echo "::error::tests pass without the product change: they do not test it"; exit 1
  fi
else
  cd "$dir"
  if pnpm exec vitest run "${tests[@]#"$dir"/}" >/tmp/fail-before.log 2>&1; then
    cat /tmp/fail-before.log
    echo "::error::tests pass without the product change: they do not test it"; exit 1
  fi
fi
tail -5 /tmp/fail-before.log
echo "pr-gate: OK, tests fail without the change"
