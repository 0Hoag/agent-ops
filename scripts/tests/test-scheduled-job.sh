#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd); tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
checks="$tmp/checks"; artifacts="$tmp/artifacts"; bin="$tmp/bin"; mkdir -p "$checks/green" "$checks/failed" "$bin"
printf '#!/usr/bin/env bash\nexit 0\n' >"$checks/pass.sh"
printf '#!/usr/bin/env bash\nprintf failure >&2\nexit 7\n' >"$checks/fail.sh"; chmod +x "$checks"/*.sh
mv "$checks/pass.sh" "$checks/green/pass.sh"
cp "$checks/fail.sh" "$checks/failed/fail.sh"
cat >"$bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${GH_CALLS:?}"
if [[ $1 == issue && $2 == list ]]; then printf '[]\n'; fi
exit 0
EOF
chmod +x "$bin/gh"; export PATH="$bin:$PATH" GH_CALLS="$tmp/gh.calls" DEFECT_REPO=fixture/repo
VERIFY_ALL_CHECK_DIR="$checks/green" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=green bash "$root/scripts/ci/scheduled-job.sh" --dry-run >/dev/null
test ! -s "$GH_CALLS"; test -s "$artifacts/green/summary.json"
set +e; VERIFY_ALL_CHECK_DIR="$checks/failed" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=failed bash "$root/scripts/ci/scheduled-job.sh" --dry-run >"$tmp/dry.out" 2>&1; rc=$?; set -e
test "$rc" = 1; grep -q 'DRY RUN:.*--check fail.*--expected exit 0.*--actual exit 7.*--evidence' "$tmp/dry.out"; test ! -s "$GH_CALLS"
set +e; VERIFY_ALL_CHECK_DIR="$checks/failed" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=production bash "$root/scripts/ci/scheduled-job.sh" >/dev/null 2>&1; rc=$?; set -e
test "$rc" = 1; test "$(wc -l <"$GH_CALLS" | tr -d ' ')" = 2
grep -q -- 'issue list --state all --search' "$GH_CALLS"; grep -q -- '--repo fixture/repo' "$GH_CALLS"
grep -q -- 'issue create --title' "$GH_CALLS"; grep -q -- 'Check: fail' "$GH_CALLS"; grep -q -- 'Actual: exit 7' "$GH_CALLS"; grep -q -- 'Evidence:' "$GH_CALLS"
mkdir "$tmp/empty"; set +e; VERIFY_ALL_CHECK_DIR="$tmp/empty" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=setup bash "$root/scripts/ci/scheduled-job.sh" >/dev/null 2>&1; rc=$?; set -e
test "$rc" = 2; test "$(wc -l <"$GH_CALLS" | tr -d ' ')" = 2
production="$tmp/production"; mkdir -p "$production/scripts/ci"
cp "$root/scripts/verify-all.sh" "$production/scripts/verify-all.sh"
cp "$root/scripts/file-defect.sh" "$production/scripts/file-defect.sh"
sed 's/exit "\$verify_rc"/exit 0/' "$root/scripts/ci/scheduled-job.sh" >"$production/scripts/ci/scheduled-job.sh"
chmod +x "$production/scripts/ci/scheduled-job.sh" "$production/scripts/verify-all.sh" "$production/scripts/file-defect.sh"
set +e; VERIFY_ALL_CHECK_DIR="$checks/failed" VERIFY_ALL_ARTIFACT_ROOT="$artifacts" VERIFY_ALL_RUN_ID=control bash "$production/scripts/ci/scheduled-job.sh" --dry-run >/dev/null 2>&1; control_rc=$?; set -e
test "$control_rc" = 0
echo 'CONTROL PASS: identical-input production exit assertion rejects forced green result'
echo 'PASS scheduled adapter dry-run, artifact, bounded filing, setup failure, and mutation control'
