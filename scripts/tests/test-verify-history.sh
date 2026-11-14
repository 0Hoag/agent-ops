#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd); tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
repo="$tmp/repo"; mkdir -p "$repo/scripts/verify-lifecycle" "$repo/scripts/verify" "$repo/.agent-runs"
cp "$root/scripts/verify-all.sh" "$repo/scripts/verify-all.sh"
cp "$root/scripts/verify-history.sh" "$repo/scripts/verify-history.sh"
cp "$root/scripts/verify-lifecycle/shared-stack.sh" "$repo/scripts/verify-lifecycle/shared-stack.sh"
printf '#!/usr/bin/env bash\nexit "${FIXTURE_STATUS:-0}"\n' >"$repo/scripts/verify/check.sh"; chmod +x "$repo"/scripts/verify/*.sh "$repo"/scripts/*.sh
(cd "$repo" && git init -q && git config user.email fixture@example.invalid && git config user.name Fixture && git add . && git commit -qm fixture)
run() { set +e; (cd "$repo" && VERIFY_ALL_RUN_ID="$1" VERIFY_ALL_ARTIFACT_ROOT="$repo/.agent-runs/verify" VERIFY_HISTORY_FILE="$repo/.agent-runs/history.json" FIXTURE_STATUS="$2" bash scripts/verify-all.sh >"$tmp/$1.out" 2>&1); rc=$?; set -e; [[ $rc == 0 || $rc == 1 ]]; }
run first 0; run second 7
python3 - "$repo/.agent-runs/verify/second/summary.json" "$repo/.agent-runs/history.json" <<'PY'
import json,sys
summary=json.load(open(sys.argv[1])); history=json.load(open(sys.argv[2]))
check=summary['checks'][0]
assert check['classification']=='flaky' and check['flaky'] is True
assert len(history['check'])==2 and history['check'][-1]['status']==7
PY
for n in $(seq 1 35); do run "retention-$n" $((n % 2)); done
python3 - "$repo/.agent-runs/history.json" <<'PY'
import json,sys
h=json.load(open(sys.argv[1])); assert len(h['check']) == 30
PY
run stable-a 7; run stable-b 7
python3 - "$repo/.agent-runs/verify/stable-b/summary.json" <<'PY'
import json,sys
c=json.load(open(sys.argv[1]))['checks'][0]; assert c['classification']=='failure' and c['flaky'] is False
PY
mutant="$tmp/mutant.sh"; sed 's/flaky = bool(same_source and same_source\[-1\].get("status") != status)/flaky = False/' "$root/scripts/verify-history.sh" >"$mutant"; chmod +x "$mutant"
set +e; VERIFY_HISTORY_FILE="$tmp/mutant-history.json" "$mutant" "$repo/.agent-runs/verify/second/summary.json" >"$tmp/mutant.out" 2>&1; rc=$?; set -e
test "$rc" = 0
if python3 - "$repo/.agent-runs/verify/second/summary.json" <<'PY'
import json,sys; assert json.load(open(sys.argv[1]))['checks'][0]['classification']=='flaky', 'same-source flip must be classified flaky'
PY
then echo 'control failed: removed same-source/flaky guard was accepted' >&2; exit 1; fi
echo 'CONTROL PASS: identical-input mutation removing same-source/flaky guard fails named assertion'
echo 'PASS production verify-all integration, 30-result retention, flaky/stable classification'
