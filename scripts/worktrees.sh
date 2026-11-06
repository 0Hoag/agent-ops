#!/usr/bin/env bash
# Worktree hygiene, no tokens. Lists every extra worktree of the given repositories and whether it is safe to remove.
#   scripts/worktrees.sh                # report (default repos: ~/agent-ops ~/agent-ops-client)
#   scripts/worktrees.sh --prune        # git worktree prune, then remove every SAFE worktree
#   scripts/worktrees.sh --remove PATH  # remove one worktree if SAFE; never forces
# SAFE = clean tree, no process running inside it, and everything on it is already in the base: HEAD is an ancestor of
# origin/reviews/development or origin/main, or equals the head of a merged PR. Anything else is reported with the reason
# (EVIDENCE, DIRTY, UNPUSHED, BUSY) and left alone. Removal uses --force only after these checks.
set -uo pipefail
mode=list; target=""
case "${1:-}" in --prune) mode=prune; shift ;; --remove) mode=remove; target=${2:?path}; shift 2 ;; esac
REPOS=("$@"); [ ${#REPOS[@]} -gt 0 ] || REPOS=("$HOME/agent-ops" "$HOME/agent-ops-client")

verdict() { # repo path head merged_heads -> prints SAFE or the reason
  local repo=$1 wt=$2 head=$3 merged=$4 evidence
  [ -d "$wt" ] || { echo MISSING; return; }
  # Never guess that ignored acceptance evidence has been archived. Keep even empty containers.
  for evidence in .agent-runs test-results; do
    [ ! -e "$wt/$evidence" ] && [ ! -L "$wt/$evidence" ] || { echo "EVIDENCE ($evidence; archive before removal)"; return; }
  done
  # Untracked build leftovers do not count as work; evidence containers were checked above.
  [ -z "$(git -C "$wt" status --porcelain 2>/dev/null | grep -Ev '^\?\? (\.pnpm-store|node_modules|dist|\.agent-runs|test-results)/?$')" ] || { echo DIRTY; return; }
  pgrep -f -- "$wt" > /dev/null && { echo BUSY; return; }
  for base in origin/reviews/development origin/main; do
    git -C "$repo" merge-base --is-ancestor "$head" "$base" 2>/dev/null && { echo SAFE; return; }
  done
  grep -qxF -- "$head" <<<"$merged" && { echo SAFE; return; }
  echo UNPUSHED
}

for repo in "${REPOS[@]}"; do
  git -C "$repo" fetch -q origin 2>/dev/null
  slug=$(git -C "$repo" remote get-url origin | sed -E 's#.*github.com[:/]##; s#\.git$##')
  merged=$(gh pr list -R "$slug" --state merged --limit 500 --json headRefOid --jq '.[].headRefOid' 2>/dev/null)
  [ "$mode" = prune ] && git -C "$repo" worktree prune
  git -C "$repo" worktree list --porcelain | awk '/^worktree /{p=substr($0,10)} /^HEAD /{print p" "$2}' | while read -r wt head; do
    [ "$wt" = "$repo" ] && continue
    [ "$mode" = remove ] && [ "$wt" != "$target" ] && continue
    v=$(verdict "$repo" "$wt" "$head" "$merged")
    case "$mode" in
      list) printf '%-9s %s\n' "$v" "$wt" ;;
      prune|remove)
        if [ "$v" = SAFE ]; then git -C "$repo" worktree remove --force "$wt" && echo "removed  $wt"
        else printf '%-9s %s (kept)\n' "$v" "$wt"; fi ;;
    esac
  done
done
