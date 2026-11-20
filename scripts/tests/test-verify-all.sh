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

fixture="$tmp/discovery-fixture"
mkdir -p "$fixture/scripts/verify-lifecycle" "$fixture/scripts/verify"
cp "$root/scripts/verify-all.sh" "$fixture/scripts/verify-all.sh"
cp "$root/scripts/verify-lifecycle/shared-stack.sh" "$fixture/scripts/verify-lifecycle/shared-stack.sh"
cp "$root/scripts/verify-history.sh" "$fixture/scripts/verify-history.sh"
cat >"$fixture/scripts/verify/resolve-previous-release.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'repository root is required\n' >&2
exit 2
EOF
cat >"$fixture/scripts/verify/feature-pass.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'feature verifier ran\n'
EOF
cat >"$fixture/scripts/verify/feature-fail.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'feature verifier failure\n' >&2
exit 7
EOF
chmod +x "$fixture/scripts/verify"/*.sh "$fixture/scripts/verify-all.sh"
set +e
(cd "$fixture" && VERIFY_ALL_ARTIFACT_ROOT="$fixture/artifacts" VERIFY_ALL_RUN_ID=discovery-fail bash scripts/verify-all.sh >"$tmp/discovery-fail.out" 2>&1)
discovery_rc=$?
set -e
test "$discovery_rc" = 1
python3 - "$fixture/artifacts/discovery-fail/summary.json" <<'PY'
import json, sys
checks = json.load(open(sys.argv[1]))['checks']
assert [(item['name'], item['status']) for item in checks] == [
    ('feature-fail', 7), ('feature-pass', 0), ('resolve-previous-release', 2)
]
PY
rm "$fixture/scripts/verify/resolve-previous-release.sh"
set +e
(cd "$fixture" && VERIFY_ALL_ARTIFACT_ROOT="$fixture/artifacts" VERIFY_ALL_RUN_ID=discovery-pass bash scripts/verify-all.sh >"$tmp/discovery-pass.out" 2>&1)
discovery_fixed_rc=$?
set -e
test "$discovery_fixed_rc" = 1
python3 - "$fixture/artifacts/discovery-pass/summary.json" <<'PY'
import json, sys
checks = json.load(open(sys.argv[1]))['checks']
assert [(item['name'], item['status']) for item in checks] == [('feature-fail', 7), ('feature-pass', 0)]
PY
grep -Fq 'feature verifier ran' "$fixture/artifacts/discovery-pass/logs/feature-pass.log"
grep -Fq 'feature verifier failure' "$fixture/artifacts/discovery-pass/logs/feature-fail.log"
echo 'PASS discovery excludes helper after relocation, runs genuine verifiers, and propagates feature failure'
echo 'CONTROL PASS identical fixture inputs fail on original helper layout and pass the fixed discovery layout'
