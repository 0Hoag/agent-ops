#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${PRICING_CONTROL_ARTIFACT_DIR:-$ROOT/.agent-runs/W19g-pricing-control-576}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/pricing-control.XXXXXX")
cleanup() {
  local rc=$?
  trap - EXIT INT TERM
  rm -rf "$TMP" "$ROOT/.agent-runs/pricing-control-readonly-fixture"
  exit "$rc"
}
trap cleanup EXIT INT TERM
mkdir -p "$ARTIFACT_DIR"

run_verifier() {
  local source=$1 mode=$2 artifact="$ARTIFACT_DIR/$2"
  mkdir -p "$artifact"
  local control=0
  [[ $mode == mutant ]] && control=1
  PRICING_VERIFY_ARTIFACT_DIR="$artifact" PRICING_VERIFY_CONTROL="$control" \
    bash "$source/scripts/verify/pricing.sh" >"$artifact/stdout.log" 2>"$artifact/stderr.log"
}

prepare_source_tree() {
  local source=$1 destination=$2
  mkdir -p "$destination"
  git -C "$source" archive --format=tar HEAD | tar -xf - -C "$destination"
  rm -rf "$destination/.agent-runs"
  git -C "$destination" init -q
  git -C "$destination" config user.email pricing-verify@example.invalid
  git -C "$destination" config user.name pricing-verify
  git -C "$destination" add -A
  git -C "$destination" commit -q -m 'fixture: pricing verifier source'
}

SOURCE_SHA=$(git -C "$ROOT" rev-parse HEAD)
printf 'source_sha=%s\n' "$SOURCE_SHA" | tee "$ARTIFACT_DIR/summary.txt"
run_verifier "$ROOT" healthy

readonly_fixture="$ROOT/.agent-runs/pricing-control-readonly-fixture"
mkdir -p "$readonly_fixture"
printf 'must remain in the original checkout\n' >"$readonly_fixture/evidence.log"
chmod 444 "$readonly_fixture/evidence.log"
before_evidence=$(shasum -a 256 "$readonly_fixture/evidence.log" | awk '{print $1}')

cp -a "$ROOT" "$TMP/legacy"
if [[ -e "$TMP/legacy/.agent-runs/pricing-control-readonly-fixture/evidence.log" ]]; then
  printf 'old_copy_control=fail (ignored evidence was copied)\n' >>"$ARTIFACT_DIR/summary.txt"
else
  echo 'old copy control did not reproduce the known defect' >&2
  exit 1
fi

prepare_source_tree "$ROOT" "$TMP/mutant"
original_git_marker=$(git -C "$ROOT" rev-parse --git-dir)
mutant_git_marker=$(git -C "$TMP/mutant" rev-parse --git-dir)
if [[ -e "$TMP/mutant/.agent-runs" || "$original_git_marker" == "$mutant_git_marker" ]]; then
  echo 'bounded source tree copied ignored evidence or original Git internals' >&2
  exit 1
fi
perl -0pi -e 's#/1e6#/1e5#g' "$TMP/mutant/api/modules/agentrun/repository.go"
MUTANT_SHA=$(sha256sum "$TMP/mutant/api/modules/agentrun/repository.go" | awk '{print $1}')
printf 'mutant_source_sha=%s\n' "$MUTANT_SHA" >>"$ARTIFACT_DIR/summary.txt"
run_verifier "$TMP/mutant" mutant
grep -Eq 'negative control failed named numeric pricing assertion' "$ARTIFACT_DIR/mutant/stderr.log" || { echo 'mutant did not fail named numeric assertion' >&2; exit 1; }
printf 'mutant_rc=0(expected-control-pass)\n' >>"$ARTIFACT_DIR/summary.txt"

run_verifier "$ROOT" restored
after_evidence=$(shasum -a 256 "$readonly_fixture/evidence.log" | awk '{print $1}')
[[ "$before_evidence" == "$after_evidence" ]] || { echo 'original ignored evidence changed' >&2; exit 1; }
printf 'healthy=pass\nmutant=expected-fail\nrestored=pass\n' >>"$ARTIFACT_DIR/summary.txt"
cat "$ARTIFACT_DIR/summary.txt"
