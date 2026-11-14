#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
stub="$tmp/docker"; log="$tmp/docker.log"
cat >"$stub" <<'EOF'
#!/usr/bin/env bash
printf '%q ' "$@" >>"$INSIGHTS_TEST_LOG"; printf '\n' >>"$INSIGHTS_TEST_LOG"
if [[ "$1" == compose && " $* " == *' down '* && ${INSIGHTS_TEST_DOWN_FAIL:-0} == 1 ]]; then exit 7; fi
if [[ "$1 $2" == "ps -aq" ]]; then printf '%s\n' "${INSIGHTS_TEST_CONTAINERS:-}"; fi
if [[ "$1 $2 $3" == "volume ls -q" ]]; then printf '%s\n' "${INSIGHTS_TEST_VOLUMES:-}"; fi
if [[ "$1 $2 $3" == "network ls -q" ]]; then printf '%s\n' "${INSIGHTS_TEST_NETWORKS:-}"; fi
EOF
chmod +x "$stub"

run() {
  : >"$log"
  set +e
  PATH="$tmp:$PATH" INSIGHTS_TEST_LOG="$log" INSIGHTS_VERIFY_CLEANUP_ONLY=1 INSIGHTS_VERIFY_ARTIFACT_DIR="$tmp/artifacts" INSIGHTS_TEST_CONTAINERS="${2:-}" INSIGHTS_TEST_VOLUMES="${3:-}" INSIGHTS_TEST_NETWORKS="${4:-}" INSIGHTS_TEST_DOWN_FAIL="${5:-0}" "$root/scripts/verify/insights.sh" >/dev/null 2>&1
  rc=$?
  set -e
  return "$rc"
}

run clean
[[ $? -eq 0 ]] || { echo 'clean standalone cleanup failed' >&2; exit 1; }
for kind in container volume network; do
  c= v= n=
  case "$kind" in container) c=owned-container ;; volume) v=owned-volume ;; network) n=owned-network ;; esac
  if run "$kind leak" "$c" "$v" "$n"; then echo "leaked $kind was accepted" >&2; exit 1; fi
  grep -Fq "FAIL: cleanup left owned ${kind}s" "$tmp/artifacts/commands.log" || { echo "missing named $kind diagnostic" >&2; exit 1; }
done
if run down-failure '' '' '' 1; then echo 'failed down was accepted' >&2; exit 1; fi

: >"$log"
PATH="$tmp:$PATH" INSIGHTS_TEST_LOG="$log" VERIFY_SHARED_STACK=1 VERIFY_SHARED_PROJECT=runner-project VERIFY_SHARED_API_PORT=1 VERIFY_SHARED_PG_PORT=2 INSIGHTS_VERIFY_CLEANUP_ONLY=1 INSIGHTS_VERIFY_ARTIFACT_DIR="$tmp/shared" INSIGHTS_TEST_CONTAINERS=owned-container "$root/scripts/verify/insights.sh" >/dev/null 2>&1
! grep -Fq 'compose down' "$log" || { echo 'shared consumer tore down runner stack' >&2; exit 1; }
! grep -Fq 'ps -aq' "$log" || { echo 'shared consumer inspected ownership' >&2; exit 1; }

mutant="$tmp/insights-mutant.sh"
sed '27s/.*/  if true; then/' "$root/scripts/verify/insights.sh" >"$mutant"
chmod +x "$mutant"
: >"$log"
set +e
PATH="$tmp:$PATH" INSIGHTS_TEST_LOG="$log" INSIGHTS_VERIFY_ROOT="$root" VERIFY_SHARED_STACK=1 VERIFY_SHARED_PROJECT=runner-project VERIFY_SHARED_API_PORT=1 VERIFY_SHARED_PG_PORT=2 INSIGHTS_VERIFY_CLEANUP_ONLY=1 INSIGHTS_VERIFY_ARTIFACT_DIR="$tmp/mutant" INSIGHTS_TEST_CONTAINERS=owned-container "$mutant" >/dev/null 2>&1
rc=$?
set -e
[[ $rc -ne 0 ]] || { echo 'guard mutation passed identical shared fixture' >&2; exit 1; }
grep -Fq 'FAIL: cleanup left owned containers' "$tmp/mutant/commands.log" || { echo 'mutation did not fail named ownership assertion' >&2; exit 1; }
echo 'PASS: standalone owned cleanup, failed down, shared consumer, and behavioral guard control'
