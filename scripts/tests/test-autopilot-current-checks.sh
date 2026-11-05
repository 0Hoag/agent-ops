#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
bin=$tmp/bin; mkdir "$bin"
cat > "$tmp/pr.json" <<'EOF'
[{"number":412,"headRefOid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","headRefName":"fix/current-checks","baseRefName":"reviews/development","isDraft":false,"updatedAt":"2026-11-05T00:00:00Z","body":"Closes #413","labels":[],"statusCheckRollup":[{"name":"PR gate","context":"CI","conclusion":"FAILURE"},{"name":"PR gate","context":"CI","conclusion":"SUCCESS"}]}]
EOF
cat > "$bin/gh" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
  'pr list')
    jq -r "${!#}" "$FIXTURE"
    ;;
  'pr checks') case "$CASE" in
    stale-success) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"SUCCESS","bucket":"pass"}]' ;;
    current-failure) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"FAILURE","bucket":"fail"}]' ;;
    pending) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"PENDING","bucket":"pending"}]' ;;
    missing) printf '%s\n' '[]' ;;
    api-error) exit 1 ;;
    malformed) printf '%s\n' '[{"name":"PR gate","state":null}]' ;;
    same-name-failure) printf '%s\n' '[{"name":"PR gate","workflow":"CI","state":"SUCCESS"},{"name":"PR gate","workflow":"external","state":"FAILURE"}]' ;;
  esac ;;
  'pr view') printf '%s\n' 'Closes #413' ;;
  'issue view') printf '%s\n' 'OPEN' ;;
  'issue close'|'issue edit') : ;;
  'pr merge') : > "$MERGED" ;;
esac
EOF
chmod +x "$bin/gh"
func=$(sed -n '/^merge_ready() {/,/^}/p' "$root/scripts/autopilot.sh")
original=$(git show origin/reviews/development:scripts/autopilot.sh | sed -n '/^merge_ready() {/,/^}/p')
run_case() {
  local case=$1 expect=$2 fn=$3
  rm -f "$tmp/merged"
  if env PATH="$bin:$PATH" REPOS=fixture/repo CASE="$case" MERGED="$tmp/merged" FIXTURE="$tmp/pr.json" bash -c '
    set -euo pipefail
    is_critical(){ return 1; }; pgrep(){ return 1; }
    '"$fn"'
    merge_ready
  ' 2>/dev/null; then rc=0; else rc=$?; fi
  local actual=false
  [[ "$rc" == 0 && -e "$tmp/merged" ]] && actual=true
  [[ "$actual" == "$expect" ]]
}
if ! run_case stale-success false "$original"; then
  echo 'CONTROL FAIL: original selector accepted stale failure' >&2; exit 1
fi
echo 'CONTROL PASS: original selector rejects stale-check eligibility on identical fixture'
run_case stale-success true "$func"
echo 'PASS stale-check eligibility: current checks selected'
for case in current-failure pending missing api-error malformed same-name-failure; do
  run_case "$case" false "$func"
  echo "PASS $case"
done
echo 'PASS fail-closed current-check regression'
