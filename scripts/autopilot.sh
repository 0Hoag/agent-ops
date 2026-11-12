#!/usr/bin/env bash
# Coordinator loop: the shell waits, the model only works.
# Each pass is a fresh, short OpenCode session (`/autopilot` = one pass of docs/coordinator-loop.md), then
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
. scripts/autopilot-state.sh
autopilot_preflight opencode || exit 2
pass_cleanup() { scripts/docker-pass-cleanup.sh; }
trap pass_cleanup EXIT
interval=${INTERVAL:-600}
poll=${POLL:-30}
cap=${CAP:-2}   # keep equal to the worker cap in docs/coordinator-loop.md → Dispatch
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

# GitHub fingerprint (gh calls only, no tokens): main SHA, open PRs with head SHA, update time and check results, latest dev CI run, open issue
# content/comments (status-board excluded: the coordinator rewrites it every pass). Taken right after a pass, so the
# pass's own changes are the baseline; any later difference (owner or CI activity, new PR) is news.
REPOS=${REPOS:-"0Hoag/agent-ops 0Hoag/agent-ops-client"}
fingerprint() {
  local r
  for r in $REPOS; do
    gh api "repos/$r/commits/main" --jq .sha || return 1
    gh api "repos/$r/commits/reviews%2Fdevelopment" --jq .sha || return 1
    gh pr list -R "$r" --json number,updatedAt,headRefOid,statusCheckRollup \
      --jq '.[] | "\(.number) \(.updatedAt) \(.headRefOid) \([.statusCheckRollup[] | .conclusion // .status] | join(","))"' || return 1
    gh run list -R "$r" --branch reviews/development --limit 1 --json status,conclusion --jq '.[] | "\(.status) \(.conclusion)"' || return 1
    gh issue list -R "$r" --state open --limit 500 --json number,labels,title,body,updatedAt \
      --jq '[.[] | select(all(.labels[]; .name != "status-board")) | {number,labels,title,body,updatedAt}] | sort_by(.number)' || return 1
    gh issue list -R "$r" --state open --limit 500 --json number,labels,comments \
      --jq '[.[] | select(all(.labels[]; .name != "status-board")) | {number,comments}] | sort_by(.number)' || return 1
  done | shasum
}
base=
pre=

# Only observed running/queued CI counts as pending; absent checks do not.
pending_ci() {
  local r prs runs
  for r in $REPOS; do
    prs=$(gh pr list -R "$r" --json statusCheckRollup --jq '[.[].statusCheckRollup[] | select((.status == "QUEUED" or .status == "IN_PROGRESS" or .status == "PENDING") or .state == "PENDING")] | length') || return 2
    runs=$(gh run list -R "$r" --branch reviews/development --limit 1 --json status --jq '[.[] | select(.status != "completed")] | length') || return 2
    case "$prs" in ''|*[!0-9]*) return 2 ;; esac
    case "$runs" in ''|*[!0-9]*) return 2 ;; esac
    [ "$((prs + runs))" -eq 0 ] || return 0
  done
  return 1
}

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

# Deterministic merge, no tokens. Flow: feature branch -> PR -> reviews/development (squash) -> promotion PR
# reviews/development -> main (merge commit, so the integration branch is never rewritten or deleted).
# A PR is merged when CI is fully green (>=1 check, none pending or failed), no worker is still pushing to its
# branch, it has been quiet for 3 min and has no `hold` label. Feature PRs also need `Closes #n` and must not be
# CRITICAL; promotion PRs are exempt (their content was reviewed when it entered reviews/development).
# After a feature merge, scripts/close-linked.sh verifies MERGED before closing linked issues. Prints one line per merge.
merge_ready() {
  local r n sha head base checks
  for r in $REPOS; do
    gh pr list -R "$r" --json number,headRefOid,headRefName,baseRefName,isDraft,updatedAt,body,labels,statusCheckRollup --jq '
      .[] | select(.isDraft | not) | select([.labels[].name] | index("hold") | not)
      | select((now - (.updatedAt | fromdateiso8601)) > 180)
      | select((.baseRefName == "reviews/development" and (.body | test("(?i)(closes|fixes|resolves) +([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)?#[0-9]+")))
               or (.baseRefName == "main" and .headRefName == "reviews/development"))
      | "\(.number) \(.headRefOid) \(.headRefName) \(.baseRefName)"' 2>/dev/null |
    while read -r n sha head base; do
      # statusCheckRollup includes superseded executions.  gh pr checks is the
      # current-head view; evaluate every row so distinct workflows/providers
      # with the same name cannot mask one another.  Any unreadable/API-error,
      # empty, pending, or failed response is ineligible (fail closed).
      checks=$(gh pr checks "$n" -R "$r" --json name,state,workflow,bucket 2>/dev/null) || continue
      jq -e 'type == "array" and length > 0 and all(.[];
        type == "object" and (.name | type == "string") and (.state | type == "string") and
        (.state == "SUCCESS" or .state == "SKIPPED" or .state == "NEUTRAL"))' \
        <<<"$checks" >/dev/null 2>&1 || continue
      if [ "$base" = main ]; then
        scripts/merge-pr.sh "$r" "$n" "$sha" merge && echo "  promoted $r#$n to main by script"
      else
        pgrep -f -- "$head" > /dev/null && continue
        is_critical "$r" "$n" "$sha" && continue
        scripts/merge-pr.sh "$r" "$n" "$sha" squash \
          && { scripts/close-linked.sh "$r" "$n" | sed 's/^/    /' && echo "  merged $r#$n into reviews/development by script"; }
      fi
    done
  done
}
MERGE_EVERY=${MERGE_EVERY:-300}
MIN_GAP=${MIN_GAP:-900}   # min seconds between passes; events (worker ended, GitHub changed) cannot wake sooner
STALL_SLEEP=${STALL_SLEEP:-3600}
notify_stalled() {
  if [ -n "${NOTIFY_CMD:-}" ]; then
    bash -c "$NOTIFY_CMD" >/dev/null 2>&1 || true
  elif command -v osascript >/dev/null 2>&1; then
    osascript -e 'display notification "No worker, pending CI, or GitHub changes observed" with title "Autopilot stalled"' >/dev/null 2>&1 || true
  fi
}
end=0

# Sleep up to $interval, but wake after at most $poll seconds once a worker ends or writes its result.
# While $cap workers run there is nothing to dispatch, so wait up to 3 x $interval (heartbeat for hung workers).
# At the heartbeat, wake for live workers/CI; otherwise stop unchanged idle backlog.
wait_for_event() {
  local pids stamp waited=0 limit=$interval now notified=0
  pids=$(worker_pids)
  stamp=$pre   # taken before the pass: a worker that ended during the pass still counts as news
  [ "$(echo $pids | wc -w)" -ge "$cap" ] && limit=$((interval * 3))
  while true; do
    while [ "$waited" -lt "$limit" ]; do
      sleep "$poll"
      waited=$((waited + poll))
      [ -e .agent-runs/STOP ] && return
      [ $((waited % MERGE_EVERY)) -lt "$poll" ] && merge_ready > /dev/stderr
      if [ "$(results_stamp)" != "$stamp" ]; then echo "  worker result written, next pass now"; return; fi
      for pid in $pids; do
        kill -0 "$pid" 2>/dev/null || { echo "  worker $pid ended, next pass now"; return; }
      done
    done
    [ -n "$(worker_pids)" ] && { echo '  live worker: bounded heartbeat, next pass now'; return; }
    now=$(fingerprint) || { echo "  API ERROR: GitHub fingerprint failed"; return 2; }
    [ "$now" != "$base" ] && { echo "  GitHub changed, next pass now"; return; }
    pending_ci; rc=$?
    [ "$rc" -ne 2 ] || { echo '  API ERROR: pending CI query failed'; return 2; }
    [ "$rc" -ne 0 ] || { echo '  pending CI: bounded heartbeat, next pass now'; return; }
    # An issue marked "in progress" with no worker running means a worker died (network, crash): never idle on that.
    stale=0; for r in $REPOS; do
      count=$(gh issue list -R "$r" --label "status:in progress" --state open --json number --jq length 2>/dev/null) || { echo "  API ERROR: GitHub issue status query failed for $r"; return 2; }
      case "$count" in ''|*[!0-9]*) echo "  API ERROR: invalid issue status count for $r"; return 2 ;; esac
      stale=$((stale + count))
    done
    [ "$stale" -gt 0 ] && { echo "  $stale issue(s) in progress but no worker: next pass now"; return; }
    echo "  STALLED: no local worker or observed pending CI; GitHub unchanged; backlog needs review"
    if [ "$notified" -eq 0 ]; then
      notify_stalled
      notified=1
    fi
    sleep "$STALL_SLEEP"
    [ -e .agent-runs/STOP ] && return
    now=$(fingerprint) || { echo "  API ERROR: GitHub fingerprint failed"; return 2; }
    [ "$now" != "$base" ] && { echo "  GitHub changed, next pass now"; return; }
  done
}

while true; do
  ts=$(date +%Y%m%d-%H%M%S)
  echo "[$ts] pass start"
  merge_ready
  scripts/worktrees.sh --prune > "$dir/worktrees.txt" 2>&1
  scripts/branches.sh --delete > "$dir/branches.txt" 2>&1; scripts/audit.sh > "$dir/audit.txt" 2>&1   # no tokens; the pass reads the two files
  pre=$(results_stamp)
  python3 scripts/autopilot-snapshot.py "$dir" $REPOS || exit 2
  # opencode v2 dropped --command/--attach/--dir: the prompt carries the instruction, cwd is the repo root.
  OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 opencode run \
    --format json \
    --title "autopilot $ts" \
    "You are the Coordinator + PM. Do exactly one pass of the loop in docs/coordinator-loop.md (read it fully once), then end your turn with the report format it describes. Do not load the orchestrator-workers skill or its references. Do not list .agent-runs/coordinator/. Read ROADMAP.md only when planning new issues. Fetch issue bodies/comments only for issues acted on in this pass. Batch several gh commands per shell call. Use the inline snapshot for sync inventory; read the status board body separately. Do not read whole JSONL transcripts. Do not sleep or wait for workers; the shell calls you again.

$(cat "$dir/snapshot.md")
" < /dev/null > "$dir/$ts.jsonl" 2> "$dir/$ts.err"
  pass_cleanup

  [ -s "$dir/$ts.err" ] && { echo "  opencode error:"; head -3 "$dir/$ts.err" | sed 's/^/    /'; }
  events_valid=1
  if ! jq -rs '[.[] | select(.type=="text")] | last | .part.text // ""' "$dir/$ts.jsonl" > "$dir/$ts.md"; then events_valid=0; : > "$dir/$ts.md"; fi
  [ -n "$(cat "$dir/$ts.md")" ] || : > "$dir/$ts.md"
  if grep -qiE 'usage limit|rate limit|quota|429|try again' "$dir/$ts.err" "$dir/$ts.jsonl" 2>/dev/null \
     && [ "$events_valid" -eq 1 ] && [ ! -s "$dir/$ts.md" ]; then
    echo "[$ts] usage limit, waiting 15 min"; sleep 900; continue
  fi
  jq -cs '[.[] | select(.type=="step_finish") | .part.tokens]
    | {input: (map(.input) | add), cached: (map(.cache.read) | add), output: (map(.output) | add)}' \
    "$dir/$ts.jsonl"
  if ! autopilot_validate_report "$dir/$ts.md"; then echo "[$ts] invalid final output; stopping safely"; exit 2; fi
  case "$(head -1 "$dir/$ts.md")" in
    'AUTOPILOT: STOP '*) echo "[$ts] coordinator asked to stop"; break ;;
  esac
  head -3 "$dir/$ts.md"
  since=$(date -u -v-24H +%Y-%m-%dT%H:%M:%S 2>/dev/null || date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%S)
  for r in $REPOS; do
    echo "  progress $r: $(gh pr list -R "$r" --state merged --search "merged:>=$since" --json number --jq length 2>/dev/null) merged/24h, $(gh issue list -R "$r" --state open --limit 500 --json number --jq length 2>/dev/null) open"
  done
  [ -e .agent-runs/STOP ] && { echo "[$ts] STOP file found"; break; }
  base=$(fingerprint) || base=
  [ -n "$base" ] || { echo "[$ts] API ERROR: GitHub fingerprint unavailable"; exit 2; }
  end=$(date +%s)
  wait_for_event || exit "$?"
  gap=$((MIN_GAP - ($(date +%s) - end))); [ "$gap" -gt 0 ] && [ ! -e .agent-runs/STOP ] && { echo "  min gap: waiting ${gap}s"; sleep "$gap"; }
done
