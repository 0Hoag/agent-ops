#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
assert_config() {
  local expected=$1 actual
  actual=$(docker compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["services"]["api"]["environment"]["SCM_REPOSITORIES_JSON"])')
  [[ "$actual" == "$expected" ]] || { printf 'SCM_REPOSITORIES_JSON mismatch: expected %s, got %s\n' "$expected" "$actual" >&2; return 1; }
  python3 -c 'import json,sys; value=json.loads(sys.argv[1]); assert isinstance(value,list)' "$actual"
}
unset SCM_REPOSITORIES_JSON
export JWT_SECRET=fixture-jwt REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901
assert_config '[]'
printf 'PASS: unset default is JSON empty array []\n'
configured='[{"provider":"github","full_name":"fixture/repo"}]'
actual=$(SCM_REPOSITORIES_JSON="$configured" JWT_SECRET=fixture-jwt REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901 docker compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["services"]["api"]["environment"]["SCM_REPOSITORIES_JSON"])')
[[ "$actual" == "$configured" ]] || { printf 'explicit JSON changed: %s\n' "$actual" >&2; exit 1; }
printf 'PASS: explicit JSON preserved byte-for-byte\n'
