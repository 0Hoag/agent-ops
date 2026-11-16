#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
event=${1:-cadence}
pass=${2:-unknown}
state=${PERIODIC_VERIFICATION_STATE:-$root/.agent-runs/coordinator/periodic-verification.state}
result=${PERIODIC_VERIFICATION_RESULT:-$root/.agent-runs/coordinator/periodic-verification.json}
every=${VERIFY_EVERY:-6}
[[ $every =~ ^[1-9][0-9]*$ ]] || every=6
mkdir -p "$(dirname "$state")" "$(dirname "$result")"
passes=0; last_event=
if [[ -f $state ]]; then IFS='|' read -r passes last_event < "$state" || true; fi
[[ $passes =~ ^[0-9]+$ ]] || passes=0
should_run=false
case "$event" in
  cadence) passes=$((passes + 1)); if (( passes % every == 0 )); then should_run=true; fi ;;
  merge:*) [[ "$event" != "$last_event" ]] && should_run=true; last_event=$event ;;
  *) echo "periodic-verification: unknown event: $event" >&2; exit 2 ;;
esac
printf '%s|%s\n' "$passes" "$last_event" > "$state"
[[ $should_run == true ]] || exit 0
run_id="periodic-${pass}-${passes}"
artifact_root=${VERIFY_ALL_ARTIFACT_ROOT:-$root/.agent-runs/verify}
set +e
VERIFY_ALL_RUN_ID="$run_id" VERIFY_ALL_ARTIFACT_ROOT="$artifact_root" bash "$root/scripts/verify-all.sh"
verify_rc=$?
set -e
summary="$artifact_root/$run_id/summary.json"
python3 - "$result" "$summary" "$event" "$pass" "$verify_rc" <<'PY'
import json, sys
from pathlib import Path
out, summary, event, passed = map(Path, sys.argv[1:5])
verify_rc = int(sys.argv[5])
checks = []
if summary.exists():
    data = json.loads(summary.read_text())
    if not isinstance(data.get('checks'), list): raise SystemExit('periodic-verification: summary checks is not a list')
    checks = [{'name': c.get('name'), 'status': c.get('status'), 'result': c.get('result')} for c in data['checks']]
out.write_text(json.dumps({'event': str(event), 'pass': str(passed), 'status': 'pass' if verify_rc == 0 else 'fail', 'exit': verify_rc, 'summary': str(summary), 'checks': checks}, indent=2) + '\n')
PY
exit 0
