#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${PRICING_CONTROL_ARTIFACT_DIR:-$ROOT/.agent-runs/W19g-pricing-control-576}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/pricing-control.XXXXXX")
trap 'rm -rf "$TMP"' EXIT INT TERM
mkdir -p "$ARTIFACT_DIR"

run_verifier() {
  local source=$1 mode=$2 artifact="$ARTIFACT_DIR/$2"
  mkdir -p "$artifact"
  local control=0
  [[ $mode == mutant ]] && control=1
  PRICING_VERIFY_ARTIFACT_DIR="$artifact" PRICING_VERIFY_CONTROL="$control" \
    bash "$source/scripts/verify/pricing.sh" >"$artifact/stdout.log" 2>"$artifact/stderr.log"
}

SOURCE_SHA=$(git -C "$ROOT" rev-parse HEAD)
printf 'source_sha=%s\n' "$SOURCE_SHA" | tee "$ARTIFACT_DIR/summary.txt"
run_verifier "$ROOT" healthy

cp -a "$ROOT" "$TMP/mutant"
perl -0pi -e 's#/1e6#/1e5#g' "$TMP/mutant/api/modules/agentrun/repository.go"
MUTANT_SHA=$(sha256sum "$TMP/mutant/api/modules/agentrun/repository.go" | awk '{print $1}')
printf 'mutant_source_sha=%s\n' "$MUTANT_SHA" >>"$ARTIFACT_DIR/summary.txt"
run_verifier "$TMP/mutant" mutant
grep -Eq 'negative control failed named numeric pricing assertion' "$ARTIFACT_DIR/mutant/stderr.log" || { echo 'mutant did not fail named numeric assertion' >&2; exit 1; }
printf 'mutant_rc=0(expected-control-pass)\n' >>"$ARTIFACT_DIR/summary.txt"

run_verifier "$ROOT" restored
printf 'healthy=pass\nmutant=expected-fail\nrestored=pass\n' >>"$ARTIFACT_DIR/summary.txt"
cat "$ARTIFACT_DIR/summary.txt"
