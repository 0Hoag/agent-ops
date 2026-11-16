#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${PRICING_CONTROL_ARTIFACT_DIR:-$ROOT/.agent-runs/pricing-control-regression-test}

PRICING_CONTROL_ARTIFACT_DIR="$ARTIFACT_DIR" \
  bash "$ROOT/scripts/verify/pricing-control-regression.sh"

test ! -e "$ROOT/.agent-runs/pricing-control-readonly-fixture"
printf 'PASS pricing control bounded-source regression and cleanup\n'
