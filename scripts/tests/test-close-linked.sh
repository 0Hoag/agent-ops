#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
bin=$tmp/bin; mkdir "$bin"
cat > "$bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1 $2" in
  'pr view')
    case "$*" in
      *'--json state,baseRefName'*)
        [ "${MODE:-}" != read-failure ] || exit 1
        printf '%s\t%s\n' "${STATE:-OPEN}" "${BASE:-reviews/development}"
        ;;
      *'--json body'*) printf '%s\n' 'Closes #464' ;;
    esac
    ;;
  'issue view') printf '%s\n' OPEN ;;
  'issue close') printf '%s\n' "$*" >> "$CLOSED" ;;
  'issue edit') : ;;
esac
EOF
chmod +x "$bin/gh"
run() { env PATH="$bin:$PATH" CLOSED="$tmp/closed" STATE="$1" BASE="$2" MODE="${3:-}" "$root/scripts/close-linked.sh" fixture/repo 508; }

if run OPEN reviews/development >"$tmp/open.out" 2>&1; then
  echo 'ASSERT FAIL: OPEN PR was accepted' >&2; exit 1
fi
test ! -e "$tmp/closed"
grep -Fq 'no issues closed' "$tmp/open.out"
echo 'CONTROL PASS: unchanged script rejects OPEN PR without closing issues'

if run MERGED reviews/development >"$tmp/merged.out" 2>&1; then :; else
  echo 'ASSERT FAIL: MERGED PR was rejected' >&2; exit 1
fi
test "$(wc -l < "$tmp/closed" | tr -d ' ')" = 1
grep -Fq -- '--comment Merged by fixture/repo#508 into reviews/development.' "$tmp/closed"
echo 'PASS MERGED PR closes linked issue with accurate base name'

: > "$tmp/closed"
if run OPEN reviews/development > /dev/null 2>&1; then
  echo 'ASSERT FAIL: OPEN control unexpectedly succeeded' >&2; exit 1
fi
test ! -s "$tmp/closed"
echo 'PASS OPEN PR closes nothing and exits nonzero'

if run MERGED main > /dev/null 2>&1; then :; else exit 1; fi
grep -Fq -- '--comment Merged by fixture/repo#508 into main.' "$tmp/closed"
echo 'PASS MERGED PR uses non-default base name'

if run OPEN reviews/development read-failure > /dev/null 2>&1; then
  echo 'ASSERT FAIL: gh PR read failure was accepted' >&2; exit 1
fi
echo 'PASS gh PR read failure exits nonzero'
