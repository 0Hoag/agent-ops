#!/usr/bin/env bash
# Coordinator loop: the shell waits, the model only works.
# Each pass is a fresh, short OpenCode session (`/autopilot` = one pass of AGENTS.md → Autopilot), then
# the shell sleeps. Workers run detached (scripts/spawn.sh) and outlive the pass.
#
#   scripts/autopilot.sh                 # pass every 10 min, or within 30 s after a worker ends
#   INTERVAL=300 scripts/autopilot.sh    # every 5 min
#   CAP=2 scripts/autopilot.sh           # with CAP workers running, timer passes are skipped (3 x INTERVAL max)
#   touch .agent-runs/STOP               # stop after the current pass
#
# If the OpenCode web server is up (scripts/opencode.sh web --port 4096), each pass runs on it
# (`--attach`) and shows live in the web UI; otherwise it runs locally. Override with WEB=<url>.
#
# Per pass: .agent-runs/coordinator/<time>.jsonl (events) and .md (final message).
# A final message whose first line starts with "AUTOPILOT: STOP" ends the loop.
set -uo pipefail

cd "$(git rev-parse --show-toplevel)"
interval=${INTERVAL:-600}
poll=${POLL:-30}
cap=${CAP:-2}   # keep equal to the worker cap in AGENTS.md → Dispatch
web=${WEB:-http://127.0.0.1:4096}
dir=.agent-runs/coordinator
mkdir -p "$dir"
rm -f .agent-runs/STOP

# Local-only checks (no network, no tokens): running worker pids and the final-message files
# (.agent-runs/<wave>.md) that run-worker*.sh writes when a worker ends.
worker_pids() { pgrep -f 'scripts/run-worker' | sort | tr '\n' ' '; }
# Workers of the sibling client repo write their results in its own .agent-runs.
RUN_DIRS=${RUN_DIRS:-".agent-runs $HOME/agent-ops-client/.agent-runs"}
results_stamp() { local d; for d in $RUN_DIRS; do stat -f '%N %m' "$d"/*.md 2>/dev/null; done | shasum; }

# GitHub fingerprint (gh calls only, no tokens): main SHA, open PRs with head SHA and update time, open issue
# numbers (status-board excluded: the coordinator rewrites it every pass). Taken right after a pass, so the
# pass's own changes are the baseline; any later difference (owner or CI activity, new PR) is news.
REPOS=${REPOS:-"0Hoag/agent-ops 0Hoag/agent-ops-client"}
fingerprint() {
  local r
  for r in $REPOS; do
    gh api "repos/$r/commits/main" --jq .sha || return 1
    gh pr list -R "$r" --json number,updatedAt,headRefOid --jq '.[] | "\(.number) \(.updatedAt) \(.headRefOid)"' || return 1
    gh issue list -R "$r" --state open --limit 500 --json number,labels \
      --jq '[.[] | select(all(.labels[]; .name != "status-board")) | .number] | sort | join(",")' || return 1
  done | shasum
}
base=
pre=

# Risk classes follow controlplane's classify:risk idea. CRITICAL = trust root, CI/automation, destructive migration:
# never script-merged; the coordinator reviews those. Everything else is merged by the script once CI is green.
CRITICAL_PATHS='^(api/main\.go|api/pkg/(auth|middleware|database|crypto)/|\.github/workflows/|scripts/(ci/|autopilot\.sh|spawn\.sh|heavy\.sh))'
is_critical() {
  local r=$1 n=$2 sha=$3 f files tmp
  files=$(gh pr view "$n" -R "$r" --json files --jq '.files[].path' 2>/dev/null) || return 0
  grep -Eq "$CRITICAL_PATHS" <<<"$files" && return 0
  tmp=$(mktemp -d)
  for f in $(grep -E '^api/migrations/.*\.sql$' <<<"$files" || true); do
    gh api -H "Accept: application/vnd.github.raw" "repos/$r/contents/$f?ref=$sha" > "$tmp/$(basename "$f")" 2>/dev/null || { rm -rf "$tmp"; return 0; }
  done
  if ls "$tmp"/*.sql > /dev/null 2>&1 && ! bash scripts/ci/classify-migration.sh "$tmp"/*.sql > /dev/null; then rm -rf "$tmp"; return 0; fi
  rm -rf "$tmp"; return 1
}

# Deterministic merge, no tokens: squash-merge every open PR whose CI is fully green (>=1 check, none pending or
# failed), whose body has `Closes #n`, that no worker is still pushing to (branch name in a running process),
# that has been quiet for 3 min and carries no `hold` label. The head SHA is pinned. Prints one line per merge.
merge_ready() {
  local r n sha branch
  for r in $REPOS; do
    gh pr list -R "$r" --json number,headRefOid,headRefName,isDraft,updatedAt,body,labels,statusCheckRollup --jq '
      .[] | select(.isDraft | not) | select([.labels[].name] | index("hold") | not)
      | select((.statusCheckRollup | length) > 0 and all(.statusCheckRollup[]; (.conclusion // "") as $c | $c == "SUCCESS" or $c == "SKIPPED" or $c == "NEUTRAL"))
      | select(.body | test("(?i)(closes|fixes|resolves) +(0Hoag/[a-z-]+)?#[0-9]+"))
      | select((now - (.updatedAt | fromdateiso8601)) > 180)
      | "\(.number) \(.headRefOid) \(.headRefName)"' 2>/dev/null |
    while read -r n sha branch; do
      pgrep -f -- "$branch" > /dev/null && continue
      is_critical "$r" "$n" "$sha" && continue
      gh pr merge "$n" -R "$r" --squash --match-head-commit "$sha" > /dev/null 2>&1 && echo "  merged $r#$n by script"
    done
  done
}
MERGE_EVERY=${MERGE_EVERY:-300}

# Sleep up to $interval, but wake after at most $poll seconds once a worker ends or writes its result.
# While $cap workers run there is nothing to dispatch, so wait up to 3 x $interval (heartbeat for hung workers).
# When the timer ends with no worker running and GitHub unchanged since the last pass, a pass would only
# report "nothing to do": keep waiting instead (a failed gh call counts as unchanged).
wait_for_event() {
  local pids stamp waited=0 limit=$interval now
  pids=$(worker_pids)
  stamp=$pre   # taken before the pass: a worker that ended during the pass still counts as news
  [ "$(echo $pids | wc -w)" -ge "$cap" ] && limit=$((interval * 3))
  while true; do
    while [ "$waited" -lt "$limit" ]; do
      sleep "$poll"
      waited=$((waited + poll))
      [ -e .agent-runs/STOP ] && return
      if [ $((waited % MERGE_EVERY)) -lt "$poll" ] && [ -n "$(merge_ready | tee /dev/stderr)" ]; then
        echo "  merged by script, next pass now"; return
      fi
      if [ "$(results_stamp)" != "$stamp" ]; then echo "  worker result written, next pass now"; return; fi
      for pid in $pids; do
        kill -0 "$pid" 2>/dev/null || { echo "  worker $pid ended, next pass now"; return; }
      done
    done
    [ -n "$(worker_pids)" ] && return
    now=$(fingerprint) || now=$base
    [ "$now" != "$base" ] && { echo "  GitHub changed, next pass now"; return; }
    echo "  idle: no worker, GitHub unchanged, skipping pass"
    waited=0
  done
}

while true; do
  ts=$(date +%Y%m%d-%H%M%S)
  echo "[$ts] pass start"
  merge_ready
  pre=$(results_stamp)
  attach=()
  if curl -s -o /dev/null -m 2 "$web"; then attach=(--attach "$web" --dir "$PWD"); echo "  on web UI $web"; fi
  OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run ${attach[@]+"${attach[@]}"} \
    --command autopilot \
    --format json \
    --title "autopilot $ts" \
    "single pass" < /dev/null > "$dir/$ts.jsonl" 2> "$dir/$ts.err"

  jq -rs '[.[] | select(.type=="text")] | last | .part.text // ""' "$dir/$ts.jsonl" > "$dir/$ts.md"
  jq -cs '[.[] | select(.type=="step_finish") | .part.tokens]
    | {input: (map(.input) | add), cached: (map(.cache.read) | add), output: (map(.output) | add)}' \
    "$dir/$ts.jsonl"
  head -3 "$dir/$ts.md"
  for r in $REPOS; do
    echo "  progress $r: $(gh pr list -R "$r" --state merged --search "merged:>=$(date -v-24H +%Y-%m-%dT%H:%M:%S)" --json number --jq length 2>/dev/null) merged/24h, $(gh issue list -R "$r" --state open --limit 500 --json number --jq length 2>/dev/null) open"
  done

  if head -1 "$dir/$ts.md" | grep -q '^AUTOPILOT: STOP'; then
    echo "[$ts] coordinator asked to stop"; break
  fi
  if grep -qiE 'usage limit|rate limit|quota|429|try again' "$dir/$ts.err" "$dir/$ts.jsonl" 2>/dev/null \
     && [ ! -s "$dir/$ts.md" ]; then
    echo "[$ts] usage limit, waiting 15 min"; sleep 900; continue
  fi
  [ -e .agent-runs/STOP ] && { echo "[$ts] STOP file found"; break; }
  base=$(fingerprint) || base=
  wait_for_event
done
