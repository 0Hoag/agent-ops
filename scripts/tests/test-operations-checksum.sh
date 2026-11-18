#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
before="$tmp/before.dump"; after="$tmp/after.dump"
cat >"$before" <<'SQL'
SET something = 'INSERT INTO fake VALUES (99, ''not a record'')';
INSERT INTO public.operations_fixture (id, value) VALUES (2, 'checksum');
INSERT INTO public.operations_fixture (id, value) VALUES (1, 'first
original');
INSERT INTO public.operations_fixture (id, value) VALUES (1, 'first
original');
SQL
cat >"$after" <<'SQL'
-- harmless pg_dump metadata variation
INSERT INTO public.operations_fixture (id, value) VALUES (1, 'first
original');
INSERT INTO public.operations_fixture (id, value) VALUES (2, 'checksum');
INSERT INTO public.operations_fixture (id, value) VALUES (1, 'first
original');
SQL
checksum() { "$root/scripts/operations-checksum.py" <"$1"; }
before_hash=$(checksum "$before"); after_hash=$(checksum "$after")
[[ "$before_hash" == "$after_hash" ]] || { printf 'FAIL: reordered complete multiline records changed canonical checksum before=%s after=%s\n' "$before_hash" "$after_hash" >&2; exit 1; }
printf 'PASS: reordered complete multiline records share checksum=%s\n' "$before_hash"
mutated="$tmp/mutated.dump"; sed "s/original/LOST-DATA/" "$after" >"$mutated"
mutated_hash=$(checksum "$mutated")
if [[ "$before_hash" == "$mutated_hash" ]]; then printf 'CONTROL FAIL: identical-input multiline data-loss mutation passed named checksum assertion\n' >&2; exit 1; fi
printf 'CONTROL PASS: identical-input multiline data-loss mutation rejected by named checksum assertion (before=%s mutated=%s)\n' "$before_hash" "$mutated_hash"
