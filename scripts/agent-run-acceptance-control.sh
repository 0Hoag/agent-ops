#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mutation=${1:?usage: $0 auth|validation|filters|pricing}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp" 2>/dev/null || true' EXIT
git -C "$ROOT" archive HEAD | tar -x -C "$tmp"
cp "$ROOT/scripts/agent-run-acceptance.sh" "$tmp/scripts/"
cp "$ROOT/docker-compose.yml" "$tmp/docker-compose.yml"
case "$mutation" in
  auth) perl -0pi -e 's/ctx\.GetHeader\("X-Agent-Run-Token"\) != c\.token/ctx.GetHeader("X-Agent-Run-Token") == c.token/' "$tmp/api/modules/agentrun/controller.go"; expected='authenticated POST: expected HTTP 201' ;;
  validation) perl -0pi -e 's/if in\.InputTokens < 0 \|\| in\.CachedInputTokens < 0 \|\| in\.CacheWriteTokens < 0 \|\| in\.OutputTokens < 0 \|\| in\.ReasoningTokens < 0/if false/' "$tmp/api/modules/agentrun/controller.go"; expected='negative token validation';;
  filters) perl -0pi -e 's/if f\.Wave != "" \{/if false \&\& f.Wave != "" {/' "$tmp/api/modules/agentrun/repository.go"; expected='repository/wave/issue filters returned wrong count';;
  pricing) perl -0pi -e 's#/1e6#/1e5#g' "$tmp/api/modules/agentrun/repository.go"; expected='aggregate exact tokens/cost mismatch';;
  *) echo "unknown mutation: $mutation" >&2; exit 2;;
esac
set +e
output=$(cd "$tmp" && AGENT_RUN_ACCEPTANCE_MUTATION=1 bash scripts/agent-run-acceptance.sh 2>&1)
status=$?
set -e
printf '%s\n' "$output"
if [[ $status -eq 0 ]]; then echo "control $mutation FAILED: mutated acceptance passed" >&2; exit 1; fi
if ! grep -Fq "$expected" <<<"$output"; then echo "control $mutation FAILED: missing diagnostic '$expected'" >&2; exit 1; fi
echo "control $mutation passed: disposable mutation failed named assertion; exit=$status"
