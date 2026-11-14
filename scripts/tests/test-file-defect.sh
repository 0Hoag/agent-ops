#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd); tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
bin=$tmp/bin; mkdir "$bin"; db=$tmp/db; comments=$tmp/comments; : >"$db"; : >"$comments"
cat >"$bin/gh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
db=${DB:?}; comments=${COMMENTS:?}; protocol_assertion=${PROTOCOL_ASSERTION:?}; repo=; next=0
for x in "$@"; do if [[ $x == --repo ]]; then next=1; elif ((next)); then repo=$x; next=0; fi; done
[[ $repo == fixture/repo ]] || { echo wrong-repo >&2; exit 1; }; cmd="$1 $2"; shift 2 || true
case "$cmd" in
  'issue list') json=; q=; while (($#)); do case $1 in --json) json=$2; shift 2;; --search) q=$2; shift 2;; *) shift;; esac; done; [[ $json == number,state,title,body ]] || { printf 'protocol assertion failed: issue list JSON fields must be number,state,title,body\n' >"$protocol_assertion"; exit 1; }; line=; while IFS= read -r row; do [[ $row == *"$q"* ]] && line=$row; done <"$db"; if [[ $line ]]; then IFS='|' read -r n st fp <<<"$line"; printf '[{"number":%s,"state":"%s","body":"Fingerprint: %s"}]\n' "$n" "$st" "$fp"; else printf '[]\n'; fi ;;
  'issue create') title=; body=; raw="$*"; while (($#)); do case $1 in --title) title=$2;; --body) body=$2;; esac; shift 2 || true; done; printf 'CREATE_ARGS|%s\n' "$raw" >>"$comments"; n=$(($(wc -l <"$db") + 1)); fp=$(sed -n 's/.*Fingerprint: \([a-f0-9]*\).*/\1/p' <<<"$body"); printf '%s|OPEN|%s\n' "$n" "$fp" >>"$db"; printf 'https://github.com/fixture/repo/issues/%s\n' "$n" ;;
  'issue comment') printf 'COMMENT|%s|%s\n' "$1" "$*" >>"$comments" ;;
  'issue reopen') n=$1; sed -i.bak "s/^$n|CLOSED|/$n|OPEN|/" "$db"; rm -f "$db.bak" ;;
  'issue view') printf '{"labels":[]}' ;;
  'issue edit') : ;;
esac
EOF
chmod +x "$bin/gh"
protocol_assertion=$tmp/protocol-assertion
run() { extra=(); [[ ${4:-} ]] && extra=(--classification "$4"); env PATH="$bin:$PATH" DB="$db" COMMENTS="$comments" PROTOCOL_ASSERTION="$protocol_assertion" FILE_DEFECT_STATE_DIR="$tmp/state" "$root/scripts/file-defect.sh" --repo fixture/repo --run run1 --feature "$1" --check "$2" --command 'make verify' --expected pass --actual "$3" --evidence /tmp/evidence.log "${extra[@]}"; }
out=$(run Login auth 'failure with token=ghp_SYNTHETIC_SECRET'); grep -q 'issues/1' <<<"$out"
out=$(run Login auth 'failure with token=ghp_SYNTHETIC_SECRET'); grep -q 'updated issue #1' <<<"$out"; test "$(grep -c '^COMMENT|' "$comments")" = 1
run Other check failure >/dev/null; printf '10\n' >"$tmp/state/run1.count"
if run Feature-11 check failure >/dev/null 2>&1; then echo 'cap failed' >&2; exit 1; fi
test "$(wc -l <"$db" | tr -d ' ')" = 2; ! grep -q ghp_SYNTHETIC_SECRET "$db"
rm -f "$tmp/state/run1.count"
run Flaky flaky 'failure' flaky; test "$(grep -c -- '--label type:ci' "$comments")" = 1
echo 'PASS create-once, repeated comment, redaction, stable normalization, cap across invocations'
broken=$tmp/broken.sh; sed 's/--json number,state,title,body /--json number,state,title,body,repository /' "$root/scripts/file-defect.sh" >"$broken"; chmod +x "$broken"
if env PATH="$bin:$PATH" DB="$db" COMMENTS="$comments" PROTOCOL_ASSERTION="$protocol_assertion" FILE_DEFECT_STATE_DIR="$tmp/control-state" "$broken" --repo fixture/repo --run run1 --feature Feature-11 --check check --command 'make verify' --expected pass --actual failure --evidence /tmp/evidence.log >/dev/null 2>&1; then echo 'control failed: restored unsupported repository field was accepted' >&2; exit 1; elif ! grep -q 'protocol assertion failed: issue list JSON fields must be number,state,title,body' "$protocol_assertion"; then echo 'control failed: unsupported JSON field did not trigger named protocol assertion' >&2; exit 1; else echo 'CONTROL PASS: identical-input unsupported repository field fails named protocol assertion'; fi
