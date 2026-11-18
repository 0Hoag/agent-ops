#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/auth-port-allocation.XXXXXX")
cleanup() { rm -rf "$TEST_ROOT"; }
trap cleanup EXIT

mkdir -p "$TEST_ROOT/bin"
cat >"$TEST_ROOT/bin/lsof" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *18123*|*15475*) exit 0;;
  *) exit 1;;
esac
EOF
chmod +x "$TEST_ROOT/bin/lsof"

run() { PATH="$TEST_ROOT/bin:$PATH" AUTH_VERIFY_ALLOCATE_ONLY=1 AUTH_VERIFY_PORT_SEED=43 bash "$1"; }
output=$(run "$ROOT/scripts/verify/auth.sh")
printf '%s\n' "$output"
grep -Fxq 'API_PORT=18124 PG_PORT=15476' <<<"$output"
printf 'PASS default allocation skips occupied candidates and keeps API/PG defaults distinct\n'

output=$(PATH="$TEST_ROOT/bin:$PATH" AUTH_VERIFY_ALLOCATE_ONLY=1 AUTH_VERIFY_API_PORT=19001 AUTH_VERIFY_PG_PORT=19002 AUTH_VERIFY_PORT_SEED=43 bash "$ROOT/scripts/verify/auth.sh")
printf '%s\n' "$output"
grep -Fxq 'API_PORT=19001 PG_PORT=19002' <<<"$output"
printf 'PASS explicit overrides remain effective\n'

control=$(mktemp "$TEST_ROOT/original.XXXXXX")
cp "$ROOT/scripts/verify/auth.sh" "$control"
perl -0pi -e 's/port_available\(\).*?API_PORT=\$\{AUTH_VERIFY_API_PORT:-\$\(allocate_port 18080\)\}\nPG_PORT=\$\{AUTH_VERIFY_PG_PORT:-\$\(allocate_port 15432\)\}/API_PORT=\${AUTH_VERIFY_API_PORT:-18123}\nPG_PORT=\${AUTH_VERIFY_PG_PORT:-15475}/s' "$control"
if output=$(run "$control" 2>&1); then
  printf '%s\n' "$output"
  printf 'original allocator unexpectedly passed occupied-port assertion\n' >&2
  exit 1
fi
grep -Fq 'API_PORT=18123 PG_PORT=15475' <<<"$output" || {
  printf '%s\n' "$output" >&2
  printf 'original allocator control failed for an unintended reason\n' >&2
  exit 1
}
printf '%s\n' "$output"
printf 'PASS original allocator fails occupied-port assertion with identical inputs\n'
