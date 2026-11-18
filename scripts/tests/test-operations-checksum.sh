#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

before="$tmp/before.dump"
after="$tmp/after.dump"
printf '%s\n' \
  "INSERT INTO public.operations_fixture (id, value) VALUES (1, 'persisted');" \
  "INSERT INTO public.operations_fixture (id, value) VALUES (2, 'checksum');" >"$before"
printf '%s\n' \
  "INSERT INTO public.operations_fixture (id, value) VALUES (2, 'checksum');" \
  "INSERT INTO public.operations_fixture (id, value) VALUES (1, 'persisted');" >"$after"

checksum() { awk '/^INSERT INTO / { print }' "$1" | LC_ALL=C sort | sha256sum | awk '{print $1}'; }
before_hash=$(checksum "$before")
after_hash=$(checksum "$after")
[[ "$before_hash" == "$after_hash" ]] || {
  printf 'FAIL: reordered persisted rows changed canonical checksum before=%s after=%s\n' "$before_hash" "$after_hash" >&2
  exit 1
}
grep -Fq 'LC_ALL=C sort | sha256sum' "$root/scripts/verify/operations.sh"
printf 'PASS: reordered identical persisted rows share checksum=%s\n' "$before_hash"

mutated="$tmp/mutated.dump"
sed '/id, value) VALUES (2/ d' "$after" >"$mutated"
mutated_hash=$(checksum "$mutated")
if [[ "$before_hash" == "$mutated_hash" ]]; then
  printf 'CONTROL FAIL: identical-input data-loss mutation passed checksum assertion\n' >&2
  exit 1
fi
printf 'CONTROL PASS: identical-input data-loss mutation rejected by named checksum assertion (before=%s mutated=%s)\n' "$before_hash" "$mutated_hash"
