#!/usr/bin/env bash

# Historical control: this is the selector from before the current-head check
# fix. Keep it local and stable so the regression remains valid after merging.
merge_ready() {
  local r n sha head base
  for r in $REPOS; do
    gh pr list -R "$r" --json number,headRefOid,headRefName,baseRefName,isDraft,updatedAt,body,labels,statusCheckRollup --jq '
      .[] | select(.isDraft | not) | select([.labels[].name] | index("hold") | not)
      | select((.statusCheckRollup | length) > 0 and all(.statusCheckRollup[]; (.conclusion // "") as $c | $c == "SUCCESS" or $c == "SKIPPED" or $c == "NEUTRAL"))
      | select((now - (.updatedAt | fromdateiso8601)) > 180)
      | select((.baseRefName == "reviews/development" and (.body | test("(?i)(closes|fixes|resolves) +([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)?#[0-9]+")))
               or (.baseRefName == "main" and .headRefName == "reviews/development"))
      | "\(.number) \(.headRefOid) \(.headRefName) \(.baseRefName)"' 2>/dev/null |
    while read -r n sha head base; do
      if [ "$base" = main ]; then
        gh pr merge "$n" -R "$r" --merge --match-head-commit "$sha" > /dev/null 2>&1 && echo "  promoted $r#$n to main by script"
      else
        pgrep -f -- "$head" > /dev/null && continue
        is_critical "$r" "$n" "$sha" && continue
        gh pr merge "$n" -R "$r" --squash --match-head-commit "$sha" > /dev/null 2>&1 \
          && { echo "  merged $r#$n into reviews/development by script"; scripts/close-linked.sh "$r" "$n" | sed 's/^/    /'; }
      fi
    done
  done
}
