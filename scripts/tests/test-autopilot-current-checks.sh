#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
bin=$tmp/bin; mkdir "$bin"
cat > "$bin/gh" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
  'pr list') printf '%s\n' '412 aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa reviews/development main' ;;
  'pr checks') case "$CASE" in
    stale-success) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"SUCCESS","bucket":"pass"}]' ;;
    current-failure) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"FAILURE","bucket":"fail"}]' ;;
    pending) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"PENDING","bucket":"pending"}]' ;;
    missing) printf '%s\n' '[]' ;;
    api-error) exit 1 ;;
    same-name-failure) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"SUCCESS"},{"name":"PR gate","workflow":"external","state":"FAILURE"}]' ;;
  esac ;;
  'pr view') printf '%s\n' 'docs/example.md' ;;
  'pr merge') : > "$MERGED" ;;
esac
EOF
chmod +x "$bin/gh"
if jq -e 'all(.[]; (.conclusion // "") == "SUCCESS" or (.conclusion // "") == "SKIPPED" or (.conclusion // "") == "NEUTRAL")' \
  <<< '[{"conclusion":"FAILURE"},{"conclusion":"SUCCESS"}]' >/dev/null 2>&1; then
  echo 'original rollup selector unexpectedly accepted stale failure' >&2; exit 1
fi
echo 'CONTROL PASS: original selector rejects identical stale-failure fixture'
func=$(sed -n '/^merge_ready() {/,/^}/p' "$root/scripts/autopilot.sh")
run_case() {
  local case=$1 expect=$2
  rm -f "$tmp/merged"
  if env PATH="$bin:$PATH" REPOS=fixture/repo CASE="$case" MERGED="$tmp/merged" bash -c '
    set -euo pipefail
    is_critical(){ return 1; }; pgrep(){ return 1; }
    '"$func"'
    merge_ready
  ' 2>/dev/null; then rc=0; else rc=$?; fi
  local actual=false
  [[ "$rc" == 0 && -e "$tmp/merged" ]] && actual=true
  [[ "$actual" == "$expect" ]]
}
run_case stale-success true
echo 'PASS stale-check eligibility: current checks selected'
for case in current-failure pending missing api-error same-name-failure; do
  run_case "$case" false
  echo "PASS $case"
done
echo 'PASS fail-closed current-check regression'
