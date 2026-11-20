#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/verify-entrypoints.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

check_entrypoints() {
  local verify_dir=$1
  local check_path
  while IFS= read -r check_path; do
    [[ -x "$check_path" ]] || {
      printf 'verifier is not executable: %s\n' "${check_path##*/}" >&2
      return 1
    }
  done < <(find "$verify_dir" -maxdepth 1 -type f -name '*.sh' -print | sort)
}

check_entrypoints "$root/scripts/verify"

mutant="$tmp/verify"
cp -R "$root/scripts/verify" "$mutant"
chmod a-x "$mutant/control.sh"
if check_entrypoints "$mutant" >"$tmp/mutant.out" 2>&1; then
  echo 'expected the non-executable control.sh mutant to fail' >&2
  exit 1
fi
grep -Fxq 'verifier is not executable: control.sh' "$tmp/mutant.out"

echo 'PASS published top-level verifier entrypoints are executable and control.sh mode regression fails'
