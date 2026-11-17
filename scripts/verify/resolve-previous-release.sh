#!/usr/bin/env bash
set -Eeuo pipefail

root=${1:?repository root is required}

if [[ -n ${PREVIOUS_RELEASE_REF:-} ]]; then
  git -C "$root" rev-parse --verify "${PREVIOUS_RELEASE_REF}^{commit}" >/dev/null 2>&1 || {
    printf 'BLOCKED: PREVIOUS_RELEASE_REF does not resolve to a Git commit: %s\n' "$PREVIOUS_RELEASE_REF" >&2
    exit 2
  }
  printf '%s\n' "$PREVIOUS_RELEASE_REF"
  exit 0
fi

main_ref=${OPERATIONS_MAIN_REF:-origin/main}
development_ref=${OPERATIONS_DEVELOPMENT_REF:-origin/reviews/development}
while IFS= read -r promotion; do
  parents=$(git -C "$root" rev-list --parents -n 1 "$promotion")
  read -r _ first_parent second_parent _ <<<"$parents"
  if [[ -n ${second_parent:-} ]] && git -C "$root" merge-base --is-ancestor "$second_parent" "$development_ref" 2>/dev/null; then
    git -C "$root" rev-parse --verify "${first_parent}^{commit}"
    exit 0
  fi
done < <(git -C "$root" log --merges --first-parent --format=%H "$main_ref" 2>/dev/null || true)

printf 'BLOCKED: no development-to-main promotion baseline could be resolved from %s\n' "$main_ref" >&2
exit 2
