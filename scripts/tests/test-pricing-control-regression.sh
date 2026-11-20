#!/usr/bin/env bash
set -Eeuo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ARTIFACT_DIR=${PRICING_CONTROL_ARTIFACT_DIR:-$ROOT/.agent-runs/pricing-control-regression-test}
TEST_TMP=$(mktemp -d "${TMPDIR:-/tmp}/pricing-control-test.XXXXXX")
OWNER_EVIDENCE="$TEST_TMP/owner-evidence"
VERIFIER_STUB="$TEST_TMP/verifier.sh"
cleanup_test() { rm -rf "$TEST_TMP"; }
trap cleanup_test EXIT INT TERM
mkdir -p "$OWNER_EVIDENCE"
printf 'owner evidence must survive\n' >"$OWNER_EVIDENCE/evidence.log"
before_evidence=$(sha256sum "$OWNER_EVIDENCE/evidence.log" | awk '{print $1}')
cat >"$VERIFIER_STUB" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
exit 0
EOF
chmod 755 "$VERIFIER_STUB"

if PRICING_CONTROL_ARTIFACT_DIR="$ARTIFACT_DIR" \
  PRICING_CONTROL_READONLY_FIXTURE="$OWNER_EVIDENCE" \
  PRICING_CONTROL_VERIFIER_SCRIPT="$VERIFIER_STUB" \
  bash "$ROOT/scripts/verify/pricing-control-regression.sh"; then
  echo 'expected refusal for pre-existing owner evidence' >&2
  exit 1
fi

after_evidence=$(sha256sum "$OWNER_EVIDENCE/evidence.log" | awk '{print $1}')
test "$before_evidence" = "$after_evidence"
test -e "$OWNER_EVIDENCE/evidence.log"
printf 'PASS pre-existing owner evidence refusal preserves identical evidence\n'

INDEPENDENT_A="$TEST_TMP/independent-a"
INDEPENDENT_B="$TEST_TMP/independent-b"
mkdir -p "$INDEPENDENT_A" "$INDEPENDENT_B"
git -C "$INDEPENDENT_A" init -q
git -C "$INDEPENDENT_B" init -q
old_a=$(git -C "$INDEPENDENT_A" rev-parse --git-dir)
old_b=$(git -C "$INDEPENDENT_B" rev-parse --git-dir)
test "$old_a" = .git
test "$old_b" = .git
if [[ "$old_a" != "$old_b" ]]; then
  echo 'old guard unexpectedly accepted independent repositories' >&2
  exit 1
fi

git_storage_identity() {
  local source=$1 common_dir
  common_dir=$(git -C "$source" rev-parse --git-common-dir)
  if [[ $common_dir != /* ]]; then
    common_dir="$source/$common_dir"
  fi
  (cd "$common_dir" && pwd -P)
}

new_a=$(git_storage_identity "$INDEPENDENT_A")
new_b=$(git_storage_identity "$INDEPENDENT_B")
test "$new_a" != "$new_b"

SHARED_MAIN="$TEST_TMP/shared-main"
SHARED_LINK="$TEST_TMP/shared-link"
mkdir -p "$SHARED_MAIN"
git -C "$SHARED_MAIN" init -q
git -C "$SHARED_MAIN" config user.email test@example.invalid
git -C "$SHARED_MAIN" config user.name test
touch "$SHARED_MAIN/file"
git -C "$SHARED_MAIN" add file
git -C "$SHARED_MAIN" commit -qm fixture
git -C "$SHARED_MAIN" worktree add -q "$SHARED_LINK"
shared_main=$(git_storage_identity "$SHARED_MAIN")
shared_link=$(git_storage_identity "$SHARED_LINK")
test "$shared_main" = "$shared_link"

printf 'PASS canonical Git identity distinguishes independent and shared repositories\n'
