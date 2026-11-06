#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/release.sh [--dry-run] [--write]

Calculate a semantic version from Conventional Commits. With --dry-run (the
default), print the proposed version and changelog without changing Git or
files. --write writes CHANGELOG.md after calculating the version.
EOF
}

dry_run=true
write=false
for arg in "$@"; do
  case "$arg" in
    --dry-run) dry_run=true ;;
    --write) dry_run=false; write=true ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

latest_tag=''
while IFS= read -r tag; do
  latest_tag=$tag
done < <(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' | sort -V)

base_version=0.1.0
range=''
if [[ -n "$latest_tag" ]]; then
  base_version=${latest_tag#v}
  range="$latest_tag..HEAD"
else
  range='HEAD'
fi

major=0
minor=0
patch=0
declare -a entries=()

while IFS= read -r sha; do
  [[ -n "${sha:-}" ]] || continue
  # Merge commits and generated merge bookkeeping are not release entries.
  (( $(git rev-list --parents -n 1 "$sha" | awk '{print NF}') > 2 )) && continue
  subject=$(git show -s --format=%s "$sha")
  body=$(git show -s --format=%b "$sha")
  [[ "$subject" == Merge\ * ]] && continue
  type=''
  conventional='^([a-z]+)(\([^)]*\))?(!)?:[[:space:]]'
  if [[ "$subject" =~ $conventional ]]; then
    type=${BASH_REMATCH[1]}
    [[ -n "${BASH_REMATCH[3]}" ]] && major=1
  else
    continue
  fi
  if [[ "$body" == *'BREAKING CHANGE:'* || "$body" == *'BREAKING-CHANGE:'* ]]; then major=1; fi
  case "$type" in
    feat) minor=1; entries+=("$subject") ;;
    fix|perf|refactor|revert) patch=1; entries+=("$subject") ;;
  esac
done < <(git log --format='%H' "$range")

IFS=. read -r major_version minor_version patch_version <<< "$base_version"
if (( major )); then
  proposed="$((major_version + 1)).0.0"
elif (( minor )); then
  proposed="$major_version.$((minor_version + 1)).0"
elif (( patch )); then
  proposed="$major_version.$minor_version.$((patch_version + 1))"
else
  proposed="$base_version"
fi

printf 'Proposed version: %s\n' "$proposed"
printf '## %s\n' "$proposed"
if ((${#entries[@]} == 0)); then
  printf '%s\n' 'No releasable changes.'
else
  for entry in "${entries[@]}"; do printf '%s\n' "- $entry"; done
fi

if "$write"; then
  {
    printf '# Changelog\n\n'
    printf '## %s\n' "$proposed"
    if ((${#entries[@]} == 0)); then printf '%s\n' 'No releasable changes.'; else for entry in "${entries[@]}"; do printf '%s\n' "- $entry"; done; fi
  } > CHANGELOG.md
fi
