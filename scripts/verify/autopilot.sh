#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

for file in \
  scripts/autopilot.sh scripts/autopilot-state.sh scripts/spawn.sh \
  scripts/merge-pr.sh scripts/close-linked.sh scripts/docker-pass-cleanup.sh \
  scripts/tests/test-verify-autopilot.sh; do
  [ -f "$file" ] || { echo "ASSERT FAIL: missing production or fixture file: $file" >&2; exit 1; }
done

test -x scripts/tests/test-verify-autopilot.sh || {
  echo 'ASSERT FAIL: dedicated verifier fixture is not executable' >&2
  exit 1
}

echo 'VERIFY autopilot: isolated real-entry-point fixtures'
bash scripts/tests/test-verify-autopilot.sh
echo 'VERIFY autopilot: all acceptance bullets passed'
