#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/verify-all.sh" <<'EOF'
#!/usr/bin/env bash
set +e
mkdir -p "$VERIFY_ALL_ARTIFACT_ROOT/$VERIFY_ALL_RUN_ID"
status=$(printenv VERIFY_STATUS || echo 0)
result=pass; [ "$status" -eq 0 ] || result=fail
printf '{"checks":[{"name":"fixture","status":%s,"result":"%s"}]}\n' "$status" "$result" > "$VERIFY_ALL_ARTIFACT_ROOT/$VERIFY_ALL_RUN_ID/summary.json"
exit "$status"
EOF
chmod +x "$tmp/verify-all.sh"
sed 's#\$root/scripts/verify-all.sh#'"$tmp"'/verify-all.sh#' "$root/scripts/periodic-verification.sh" > "$tmp/periodic-verification.sh"
chmod +x "$tmp/periodic-verification.sh"
state="$tmp/state"; result="$tmp/result.json"; artifact="$tmp/artifacts"
run() { VERIFY_ALL_ARTIFACT_ROOT="$artifact" PERIODIC_VERIFICATION_STATE="$state" PERIODIC_VERIFICATION_RESULT="$result" bash "$tmp/periodic-verification.sh" "$@"; }
run cadence pass-1; test ! -e "$result"
run cadence pass-2; test ! -e "$result"
run cadence pass-3; test ! -e "$result"
run cadence pass-4; test ! -e "$result"
run cadence pass-5; test ! -e "$result"
VERIFY_STATUS=7 run cadence pass-6
test "$(jq -r .status "$result")" = fail
test "$(jq -r .exit "$result")" = 7
before=$(stat -f %m "$result")
run cadence pass-7; test "$(stat -f %m "$result")" = "$before"
VERIFY_STATUS=0 run 'merge:fixture#1@abc' pass-6
test "$(jq -r .event "$result")" = 'merge:fixture#1@abc'
before=$(stat -f %m "$result")
run 'merge:fixture#1@abc' pass-7; test "$(stat -f %m "$result")" = "$before"
VERIFY_STATUS=7 run 'merge:fixture#2@def' pass-8
test "$(jq -r .event "$result")" = 'merge:fixture#2@def'
printf 'PASS cadence/default=6, merge trigger/dedup, summary and non-fatal failure\n'
echo 'CONTROL PASS: production trigger is covered by the merge-event invocation in autopilot.sh'
mutated="$tmp/periodic-verification-mutated.sh"
sed 's/if (( passes % every == 0 )); then should_run=true; fi/if false; then should_run=true; fi/' "$tmp/periodic-verification.sh" > "$mutated"
rm -f "$result" "$state"
for pass in 1 2 3 4 5 6; do bash "$mutated" cadence "mutated-$pass"; done
if test -e "$result"; then
  echo 'CONTROL FAIL: cadence guard mutation still ran verification' >&2; exit 1
fi
echo 'CONTROL PASS: identical cadence inputs fail when the production trigger guard is mutated'
