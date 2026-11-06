#!/usr/bin/env bash
# After a squash merge: is the PR's content really on the target branch? (git log cannot tell: squash makes new commits.)
#   scripts/check-landed.sh <target-branch> <pr-branch>      e.g. reviews/development feat/w9-x
# Compares only the files the PR touched, from the merge base. Prints the lines the PR branch has and the target lacks
# (a `+` in `git diff target branch`), i.e. commits pushed after the merge or a merge that dropped content.
# Silent, exit 0: landed (or the branch is already deleted). Exit 1: something did not land.
set -uo pipefail
target=${1:?target}; branch=${2:?branch}
git fetch -q origin "$target" "$branch" 2>/dev/null || { echo "branch $branch is gone, nothing to compare"; exit 0; }
files=$(git diff --name-only "origin/$target...origin/$branch")
[ -n "$files" ] || exit 0
# shellcheck disable=SC2086
missing=$(git diff "origin/$target" "origin/$branch" -- $files | grep -E '^\+[^+]' | head -20)
[ -z "$missing" ] && exit 0
echo "NOT LANDED: $branch has content $target lacks (first lines):"; echo "$missing"; exit 1
