#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

fake_heavy="$tmp/heavy.sh"; log="$tmp/heavy.log"
cat >"$fake_heavy" <<'EOF'
#!/usr/bin/env bash
printf '%q ' "$@" >>"$VERIFY_SHARED_TEST_LOG"; printf '\n' >>"$VERIFY_SHARED_TEST_LOG"
exit 0
EOF
chmod +x "$fake_heavy"
context="$tmp/context.env"
VERIFY_SHARED_TEST_LOG="$log" VERIFY_SHARED_HEAVY="$fake_heavy" bash -c '
  source "$1/scripts/verify-lifecycle/shared-stack.sh"
  shared_stack_start "$1" "$2"
  shared_stack_cleanup "$1" "agent-ops-shared-test" "$2"
' _ "$root" "$context"
grep -Fq 'up -d --build api' "$log"
grep -Fq 'REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901' "$log"
grep -Fq 'down --volumes --remove-orphans' "$log"
grep -Fq 'VERIFY_SHARED_STACK' "$root/scripts/verify/insights.sh"
grep -Fq 'VERIFY_SHARED_STACK' "$root/scripts/verify/pricing.sh"
grep -Fq 'shared_stack_cleanup' "$root/scripts/verify-all.sh"

assert_reuse() { grep -Fq 'VERIFY_SHARED_STACK' "$1"; }
assert_cleanup_context() { grep -Fq 'REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901' "$1"; }
cp "$root/scripts/verify/pricing.sh" "$tmp/mutant.sh"
sed -i.bak '/VERIFY_SHARED_STACK/d' "$tmp/mutant.sh"
if assert_reuse "$tmp/mutant.sh"; then
  echo 'control failed: mutation disabling shared-stack reuse passed' >&2
  exit 1
fi
echo 'CONTROL PASS: identical-input mutation disabling reuse failed named lifecycle assertion'
cp "$root/scripts/verify-lifecycle/shared-stack.sh" "$tmp/shared-stack-mutant.sh"
sed -i.bak '/REPOSITORY_ENCRYPTION_KEY=/d' "$tmp/shared-stack-mutant.sh"
mutant_log="$tmp/mutant-heavy.log"
set +e
VERIFY_SHARED_TEST_LOG="$mutant_log" VERIFY_SHARED_HEAVY="$fake_heavy" bash -c '
  source "$1"
  shared_stack_start "$2" "$3"
' _ "$tmp/shared-stack-mutant.sh" "$root" "$tmp/mutant-context" >/dev/null 2>&1
mutant_helper_rc=$?
set -e
[[ $mutant_helper_rc -eq 0 ]] || { echo "control setup failed: mutated helper rc=$mutant_helper_rc" >&2; exit 1; }
if grep -Fq 'REPOSITORY_ENCRYPTION_KEY=01234567890123456789012345678901' "$mutant_log"; then
  echo 'control failed: mutation removing cleanup/start context passed' >&2
  exit 1
fi
echo 'CONTROL PASS: identical-input mutation removing explicit context failed runtime context assertion'
echo 'PASS: explicit shared context, two opt-in consumers, runner-owned lifecycle'
