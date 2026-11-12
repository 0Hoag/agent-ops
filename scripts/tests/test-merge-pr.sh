#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
bin=$tmp/bin; mkdir "$bin"
cat > "$bin/gh" <<'EOF'
#!/usr/bin/env bash
set -e
log=${LOG:?}; count=${COUNT:?}; mode=${MODE:?}
case "$1 $2" in
  'pr view')
    if [[ " $* " == *' --json state,headRefOid '* ]]; then
      printf 'OPEN\t%s\n' "${HEAD:-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
    elif [[ " $* " == *' --json state,baseRefName '* ]]; then
      if [[ "${MODE:-}" = success || "${MODE:-}" = promotion ]] && [[ -s "$count" ]]; then printf 'MERGED\treviews/development\n'; else printf '%s\treviews/development\n' "${STATE:-OPEN}"; fi
    elif [[ " $* " == *' --json state '* ]]; then
      if [[ "${MODE:-}" = success || "${MODE:-}" = promotion ]] && [[ -s "$count" ]]; then printf 'MERGED\n'; else printf '%s\n' "${STATE:-OPEN}"; fi
    elif [[ " $* " == *' --json body '* ]]; then
      printf '%s\n' 'Closes #419'
    fi ;;
  'pr merge')
    n=$(($(cat "$count" 2>/dev/null || echo 0)+1)); echo "$n" > "$count"
    echo "$*" >> "$log"
    case "$mode:$n" in
      success:2|open:1|promotion:1) exit 0 ;;
      success:1) echo '502 Bad Gateway' >&2; exit 1 ;;
      client:1) echo '422 Unprocessable Entity' >&2; exit 1 ;;
      fail:1|fail:2|fail:3) echo '502 Bad Gateway' >&2; exit 1 ;;
    esac ;;
  'issue view') echo OPEN ;;
  'issue close') echo closed >> "$CLOSED" ;;
  'issue edit') : ;;
esac
EOF
chmod +x "$bin/gh"
sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
run() { rm -f "$tmp/count" "$tmp/log" "$tmp/closed"; env PATH="$bin:$PATH" LOG="$tmp/log" COUNT="$tmp/count" CLOSED="$tmp/closed" MODE="$1" STATE="${2:-OPEN}" "$root/scripts/merge-pr.sh" fixture/repo 419 "$sha" "$3"; }

run success OPEN squash
test "$(cat "$tmp/count")" = 2
grep -q -- '--squash --match-head-commit' "$tmp/log"
env PATH="$bin:$PATH" LOG="$tmp/log" COUNT="$tmp/count" CLOSED="$tmp/closed" MODE=success "$root/scripts/close-linked.sh" fixture/repo 419 >/dev/null
test "$(wc -l < "$tmp/closed" | tr -d ' ')" = 1
echo 'PASS 502/success fixture protocol (single confirmed merge)'

if run fail OPEN squash 2>/dev/null; then exit 1; fi
test "$(cat "$tmp/count")" = 3
test ! -e "$tmp/closed"
echo 'PASS three 502s are nonzero with no closure'

rm -f "$tmp/count"
if env PATH="$bin:$PATH" LOG="$tmp/log" COUNT="$tmp/count" CLOSED="$tmp/closed" MODE=client "$root/scripts/merge-pr.sh" fixture/repo 419 "$sha" squash 2>/dev/null; then exit 1; fi
test "$(cat "$tmp/count")" = 1
echo 'PASS 4xx merge failure does not retry'

rm -f "$tmp/count"
if env PATH="$bin:$PATH" LOG="$tmp/log" COUNT="$tmp/count" CLOSED="$tmp/closed" MODE=success HEAD=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb "$root/scripts/merge-pr.sh" fixture/repo 419 "$sha" squash 2>/dev/null; then exit 1; fi
test "$(cat "$tmp/count" 2>/dev/null || echo 0)" = 0
echo 'PASS head mismatch does not retry'

# The fixture's zero-exit/open case is explicit and must not be treated as success.
STATE=OPEN
if env PATH="$bin:$PATH" LOG="$tmp/log" COUNT="$tmp/count" CLOSED="$tmp/closed" MODE=open STATE=OPEN "$root/scripts/merge-pr.sh" fixture/repo 419 "$sha" squash 2>/dev/null; then exit 1; fi
test ! -e "$tmp/closed"
echo 'PASS zero-exit merge with OPEN state fails closed'

run promotion OPEN merge
grep -q -- '--merge --match-head-commit' "$tmp/log"
echo 'PASS promotion retains merge mode'

broken=$tmp/merge-pr-broken.sh
sed 's/\[ "$final_state" = MERGED \]/true/' "$root/scripts/merge-pr.sh" > "$broken"
chmod +x "$broken"
if env PATH="$bin:$PATH" LOG="$tmp/log" COUNT="$tmp/count" CLOSED="$tmp/closed" MODE=open STATE=OPEN "$broken" fixture/repo 419 "$sha" squash >/dev/null 2>&1; then
  echo 'CONTROL PASS: removing MERGED guard makes the identical OPEN-state assertion fail'
else
  echo 'CONTROL FAIL: mutation did not demonstrate the intended assertion' >&2; exit 1
fi
