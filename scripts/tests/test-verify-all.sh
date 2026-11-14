#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd); tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
checks="$tmp/checks"; artifacts="$tmp/artifacts"; mkdir "$checks"
printf '%s\n' '#!/usr/bin/env bash' 'printf "fixture pass\\n"' >"$checks/pass.sh"
printf '%s\n' '#!/usr/bin/env bash' 'printf "fixture failure\\n" >&2' 'exit 7' >"$checks/named-failure.sh"
chmod +x "$checks"/*.sh
set +e
VERIFY_ALL_CHECK_DIR="$checks" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=fixture-fail bash "$root/scripts/verify-all.sh" >"$tmp/fail.out" 2>&1
rc=$?; set -e; test "$rc" = 1
python3 - "$artifacts/fixture-fail/summary.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1])); assert [(x['name'],x['status']) for x in d['checks']]==[('named-failure',7),('pass',0)]
PY
grep -q 'named-failure.*7' "$artifacts/fixture-fail/table.txt"
grep -q 'fixture failure' "$artifacts/fixture-fail/logs/named-failure.log"
mkdir "$tmp/pass-checks"; cp "$checks/pass.sh" "$tmp/pass-checks/only-pass.sh"
VERIFY_ALL_CHECK_DIR="$tmp/pass-checks" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=fixture-pass bash "$root/scripts/verify-all.sh" >"$tmp/pass.out"
set +e
VERIFY_ALL_CHECK_DIR="$checks" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=control bash <(sed 's/exit "$overall"/exit 0/' "$root/scripts/verify-all.sh") >/dev/null 2>&1
mutated_rc=$?
set -e
test "$mutated_rc" = 0 || { echo "control setup failed: mutation rc=$mutated_rc" >&2; exit 1; }
echo "CONTROL PASS: identical-input assertion rejects restored expected rc=1 (mutation returned $mutated_rc)"
echo 'PASS stub pass/fail aggregation, exact statuses, JSON, all-pass result'
