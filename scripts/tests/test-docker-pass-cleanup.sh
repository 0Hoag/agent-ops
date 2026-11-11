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
  ps) printf '%s\n' \
    'stopped-w97 w97-build' \
    'stopped-run agent-run-123' \
    'stopped-watch agent-watch-123' \
    'stopped-smoke agent-ops-smoke-123' \
    'running-w97 w97-running' \
    'stopped-unrelated unrelated-project' \
    'stopped-unlabelled ' ;;
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

DOCKER_LOG=$tmp/docker.log PATH="$tmp/bin:$PATH" bash "$ROOT/scripts/docker-pass-cleanup.sh"
grep -Fxq 'builder prune -f --filter until=24h' "$tmp/docker.log"
grep -Fxq 'ps -a --filter status=exited --filter label=com.docker.compose.project --format {{.ID}} {{.Label "com.docker.compose.project"}}' "$tmp/docker.log"
for id in stopped-w97 stopped-run stopped-watch stopped-smoke; do grep -Fxq "rm $id" "$tmp/docker.log"; done
for id in running-w97 stopped-unrelated stopped-unlabelled; do ! grep -Fq "rm $id" "$tmp/docker.log"; done
! grep -Eq 'volume|system prune|image prune|container prune' "$tmp/docker.log"

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
