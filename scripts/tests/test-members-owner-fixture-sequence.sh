#!/usr/bin/env bash
set -Eeuo pipefail

# Execute the fixture sequence against a tiny SQL/HTTP model. The negative
# control keeps identical HTTP inputs but uses the old second-owner insertion
# before the first rejection checks; it must fail because those checks are no
# longer sole-owner checks.
TMP=$(mktemp -d "${TMPDIR:-/tmp}/members-owner-sequence.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

run_case() {
  local second_owner_first=$1 expected=$2 state="$TMP/owners"
  printf '1\n' >"$state"
  local code=0
  owner_count() { cat "$state"; }
  insert_second_owner() { printf '2\n' >"$state"; }
  request() {
    local method=$1 path=$2 body=${3:-}
    case "$method $path $body" in
      "PATCH /api/v1/organizations/org/members/owner {\"role\":\"member\"}") CODE=$([[ $(owner_count) == 1 ]] && echo 409 || echo 204) ;;
      "DELETE /api/v1/organizations/org/members/owner ") CODE=$([[ $(owner_count) == 1 ]] && echo 409 || echo 204) ;;
      "PATCH /api/v1/organizations/org/members/admin {\"role\":\"member\"}") CODE=204; printf '1\n' >"$state" ;;
      *) CODE=204 ;;
    esac
  }
  check() { [[ "$3" == "$2" ]] || { printf 'FAIL %s: got %s, want %s\n' "$1" "$3" "$2" >&2; return 1; }; }
  if [[ "$second_owner_first" == yes ]]; then insert_second_owner; fi
  request PATCH /api/v1/organizations/org/members/owner '{"role":"member"}'; check 'sole owner demotion rejected' 409 "$CODE" || code=$?
  request DELETE /api/v1/organizations/org/members/owner; check 'sole owner removal rejected' 409 "$CODE" || code=$?
  if [[ $code -eq 0 && "$second_owner_first" == no ]]; then
    insert_second_owner; [[ $(owner_count) == 2 ]] || code=1
    request PATCH /api/v1/organizations/org/members/admin '{"role":"member"}'; check 'owner demotes second owner' 204 "$CODE" || code=$?
    [[ $(owner_count) == 1 ]] || code=1
    request PATCH /api/v1/organizations/org/members/owner '{"role":"member"}'; check 'last owner demotion rejected' 409 "$CODE" || code=$?
  fi
  [[ "$expected" == pass && $code -eq 0 ]] || [[ "$expected" == fail && $code -ne 0 ]]
}
run_case no pass
run_case yes fail
printf 'PASS members owner fixture sequence and identical-input control\n'
