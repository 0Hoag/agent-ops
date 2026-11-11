#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/docker-pass-cleanup.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/state"

cat >"$tmp/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -uo pipefail
printf '%s\n' "$*" >>"${DOCKER_LOG:?}"
case "${1:-}" in
  builder) exit 0 ;;
  ps)
    status_filter= label_filter=false
    for arg in "$@"; do
      case "$arg" in
        status=*) status_filter=${arg#status=} ;;
        label=com.docker.compose.project) label_filter=true ;;
      esac
    done
    while read -r id status project; do
      [ -z "$status_filter" ] || [ "$status" = "$status_filter" ] || continue
      [ "$label_filter" = false ] || [ -n "$project" ] || continue
      printf '%s %s\n' "$id" "$project"
    done <<'FIXTURES'
stopped-w97 exited w97-build
stopped-run exited agent-run-123
stopped-watch exited agent-watch-123
stopped-smoke exited agent-ops-smoke-123
running-w97 running w97-running
stopped-unrelated exited unrelated-project
stopped-unlabelled exited
FIXTURES
    ;;
  rm)
    id=${2:-}
    case "$id" in
      stopped-w97|stopped-run|stopped-watch|stopped-smoke) : ;;
      *) printf 'unexpected removal: %s\n' "$id" >&2; exit 1 ;;
    esac ;;
  *) printf 'unexpected docker command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$tmp/bin/docker"

assert_cleanup() {
  local id
  for id in running-w97 stopped-unrelated stopped-unlabelled; do
    if grep -Fxq "rm $id" "$tmp/docker.log"; then
      echo "FAIL: forbidden removal: $id" >&2
      return 1
    fi
  done
  if grep -Eq 'volume|system prune|image prune|container prune' "$tmp/docker.log"; then
    echo 'FAIL: forbidden prune or volume command' >&2
    return 1
  fi
  grep -Fxq 'builder prune -f --filter until=24h' "$tmp/docker.log" || return 1
  grep -Fxq 'ps -a --filter status=exited --filter label=com.docker.compose.project --format {{.ID}} {{.Label "com.docker.compose.project"}}' "$tmp/docker.log" || return 1
  for id in stopped-w97 stopped-run stopped-watch stopped-smoke; do
    grep -Fxq "rm $id" "$tmp/docker.log" || return 1
  done
}

run_cleanup() {
  : >"$tmp/docker.log"
  DOCKER_LOG=$tmp/docker.log PATH="$tmp/bin:$PATH" bash "$1" || return 1
  assert_cleanup
}

run_cleanup "$ROOT/scripts/docker-pass-cleanup.sh"

# Mutate each real guard separately; the stub and fixtures stay identical.
sed 's/--filter status=exited //' "$ROOT/scripts/docker-pass-cleanup.sh" >"$tmp/no-status.sh"
sed 's/w97\*|agent-run-\*|agent-watch-\*|agent-ops-smoke-\*)/*)/' "$ROOT/scripts/docker-pass-cleanup.sh" >"$tmp/no-allowlist.sh"
for control in no-status no-allowlist; do
  result=0
  run_cleanup "$tmp/$control.sh" >"$tmp/$control.log" 2>&1 || result=$?
  case "$control" in
    no-status) expected='FAIL: forbidden removal: running-w97' ;;
    no-allowlist) expected='FAIL: forbidden removal: stopped-unrelated' ;;
  esac
  if [ "$result" -ne 1 ] || ! grep -Fxq "$expected" "$tmp/$control.log"; then
    echo "FAIL: $control control did not fail at its safety assertion (exit $result)" >&2
    cat "$tmp/$control.log" >&2
    exit 1
  fi
  echo "PASS: $control control: $expected (exit $result)"
done
run_cleanup "$ROOT/scripts/docker-pass-cleanup.sh"
echo 'PASS: original helper before and after identical-input controls'

# Docker unavailable is a successful no-op and does not retry.
no_docker="$tmp/no-docker"; mkdir "$no_docker"
PATH="$no_docker:/usr/bin:/bin" bash "$ROOT/scripts/docker-pass-cleanup.sh"

# The production registration must invoke the real helper after every model pass.
grep -Fqx '  pass_cleanup' <(sed -n '/opencode run/,/pass_cleanup/p' "$ROOT/scripts/autopilot.sh" | tail -1)

# Identical-input control: removing the production registration must fail the same assertion.
mutated="$tmp/autopilot.sh"
cp "$ROOT/scripts/autopilot.sh" "$mutated"
perl -0pi -e 's/^  pass_cleanup\n//m' "$mutated"
if grep -Fqx '  pass_cleanup' <(sed -n '/opencode run/,/pass_cleanup/p' "$mutated" | tail -1); then
  echo 'FAIL: registration control unexpectedly passed' >&2
  exit 1
fi
echo 'PASS: bounded cleanup args, allowlist, stopped-state control, unavailable Docker, and registration control'
