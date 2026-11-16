#!/usr/bin/env bash
set -Eeuo pipefail

# Regression for the verifier's token command substitutions. The HTTP inputs are
# identical; only the diagnostic stream is mutated in the negative control.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/members-verifier-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

extract_check() {
  sed -n '/^check()/,/^json()/p' "$1" | sed '$d'
}

run_case() {
  local verifier=$1 expected=$2 output
  status_file="$TMP/body"
  TMP="$TMP"
  printf '%s\n' '{}' >"$status_file"
  log() { printf 'members: %s\n' "$*"; }
  # shellcheck disable=SC1090
  eval "$(extract_check "$verifier")"
  output=$(check 'login fixture@example.invalid' 200 200 2>/dev/null)
  [[ "$output" == "$expected" ]] || {
    printf 'unexpected command-substitution output: %q (expected %q)\n' "$output" "$expected" >&2
    return 1
  }
}

baseline="$TMP/members.sh"
cp "$ROOT/scripts/verify/members.sh" "$baseline"
run_case "$baseline" ''

mutant="$TMP/members-mutant.sh"
sed 's/" >&2;/";/' "$baseline" >"$mutant"
if run_case "$mutant" 'members: PASS: login fixture@example.invalid HTTP 200'; then
  :
else
  printf 'negative control did not reproduce diagnostic leakage\n' >&2
  exit 1
fi

printf 'PASS members verifier token-output regression and identical-input control\n'
