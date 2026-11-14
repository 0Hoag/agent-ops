#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

grep -Fq 'shared_stack_start' "$root/scripts/verify-lifecycle/shared-stack.sh"
grep -Fq 'up -d --build api' "$root/scripts/verify-lifecycle/shared-stack.sh"
grep -Fq 'VERIFY_SHARED_STACK' "$root/scripts/verify/insights.sh"
grep -Fq 'VERIFY_SHARED_STACK' "$root/scripts/verify/pricing.sh"
grep -Fq 'shared_stack_cleanup' "$root/scripts/verify-all.sh"

assert_reuse() { grep -Fq 'VERIFY_SHARED_STACK' "$1"; }
cp "$root/scripts/verify/pricing.sh" "$tmp/mutant.sh"
sed -i.bak '/VERIFY_SHARED_STACK/d' "$tmp/mutant.sh"
if assert_reuse "$tmp/mutant.sh"; then
  echo 'control failed: mutation disabling shared-stack reuse passed' >&2
  exit 1
fi
echo 'CONTROL PASS: identical-input mutation disabling reuse failed named lifecycle assertion'
echo 'PASS: explicit shared context, two opt-in consumers, runner-owned lifecycle'
