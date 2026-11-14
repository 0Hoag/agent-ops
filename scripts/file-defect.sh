#!/usr/bin/env bash
set -Eeuo pipefail
# Usage: file-defect.sh --repo OWNER/REPO --run RUN_ID --feature F --check C --command CMD --expected TEXT --actual TEXT --evidence PATH [--area AREA]
die() { printf 'file-defect: %s\n' "$*" >&2; exit 1; }
repo= run_id= feature= check_name= command= expected= actual= evidence= area=ci
while (($#)); do
  case $1 in
    --repo|--run|--feature|--check|--command|--expected|--actual|--evidence|--area)
      (($# >= 2)) || die "missing value for $1"; key=${1#--}; [[ $key == check ]] && key=check_name; [[ $key == run ]] && key=run_id; printf -v "$key" '%s' "$2"; shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done
[[ $repo && $run_id && $feature && $check_name && $command && $expected && $actual && $evidence ]] || die 'all required options must be provided'
[[ $repo =~ ^[^/[:space:]]+/[^/[:space:]]+$ ]] || die 'invalid repository identity'
[[ $run_id =~ ^[A-Za-z0-9._-]+$ ]] || die 'invalid run id'
redact() { sed -E 's/(gh[pousr]_[A-Za-z0-9_+-]+|github_pat_[A-Za-z0-9_+-]+|sk-[A-Za-z0-9_-]+|eyJ[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+|Bearer[[:space:]]+[A-Za-z0-9._~+\/-]+=*|([Pp]ass(word)?|[Tt]oken|[Ss]ecret|[Aa]pi[-_]?key)[[:space:]]*[:=][[:space:]]*[^[:space:],;]+)/[REDACTED]/g'; }
norm() { redact <<<"$1" | tr '\n\r\t' '   ' | sed -E 's/[[:space:]]+/ /g; s/^ //; s/ $//' | tr '[:upper:]' '[:lower:]'; }
safe_feature=$(redact <<<"$feature"); safe_check=$(redact <<<"$check_name"); safe_command=$(redact <<<"$command"); safe_expected=$(redact <<<"$expected"); safe_actual=$(redact <<<"$actual"); safe_evidence=$(redact <<<"$evidence")
message=$(norm "$actual")
fingerprint=$(printf '%s\037%s\037%s' "$(norm "$feature")" "$(norm "$check_name")" "$message" | openssl dgst -sha256 -r | awk '{print $1}')
gh() { command gh "$@" --repo "$repo"; }
state_root=${FILE_DEFECT_STATE_DIR:-"$(git rev-parse --git-common-dir 2>/dev/null)/file-defect"}; mkdir -p "$state_root"; state_file="$state_root/$run_id.count"
count=0; [[ -f $state_file ]] && read -r count <"$state_file" || true; [[ $count =~ ^[0-9]+$ ]] || die 'invalid run state'
issues=$(gh issue list --state all --search "$fingerprint" --json number,state,title,body,repository 2>/dev/null) || die 'GitHub search failed; no issue created'
issue=$(node -e 'const x=JSON.parse(process.argv[1]); const f=process.argv[2]; const a=x.filter(i=>JSON.stringify(i).includes(f)); if(a.length) process.stdout.write(String(a[0].number))' "$issues" "$fingerprint") || die 'invalid GitHub search response'
if [[ $issue ]]; then
  state=$(node -e 'const x=JSON.parse(process.argv[1]); const n=process.argv[2]; const i=x.find(v=>String(v.number)===n); process.stdout.write(i?.state||"")' "$issues" "$issue")
  body="Recurring failure [$fingerprint]\nFeature: $safe_feature\nCheck: $safe_check\nActual: $safe_actual\nEvidence: $safe_evidence"
  gh issue comment "$issue" --body "$body" >/dev/null || die 'GitHub comment failed; no state changed'
  if [[ $state == CLOSED ]]; then gh issue reopen "$issue" >/dev/null || die 'GitHub reopen failed'; GH_ARGS="--repo $repo" node scripts/issue-status.mjs "$issue" todo >/dev/null || die 'status update failed'; fi
  printf 'updated issue #%s (%s)\n' "$issue" "$state"; exit 0
fi
(( count < 10 )) || die 'run cap of 10 newly filed issues reached'
title="[$safe_feature] $safe_check failed ($fingerprint)"; body="Found by automated verification.\n\nFeature: $safe_feature\nCheck: $safe_check\nCommand: $safe_command\nExpected: $safe_expected\nActual: $safe_actual\nEvidence: $safe_evidence\nFingerprint: $fingerprint"
new_issue=$(gh issue create --title "$title" --body "$body" --label found-by:agent --label type:bug --label wave:W19 --label area:"$area" --label priority:P2 --label status:todo) || die 'GitHub create failed'
printf '%s\n' "$((count + 1))" >"$state_file"; printf '%s\n' "$new_issue"
