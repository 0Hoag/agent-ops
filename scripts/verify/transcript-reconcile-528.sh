#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
if [[ "${1:-}" == "--self-test" ]]; then
  (cd "$ROOT" && go run ./scripts/verify/transcript-reconcile-528.go --self-test)
else
  (cd "$ROOT" && go run ./scripts/verify/transcript-reconcile-528.go "$@")
fi
