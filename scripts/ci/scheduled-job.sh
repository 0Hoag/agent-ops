#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
dry_run=false
if [[ ${1:-} == --dry-run ]]; then dry_run=true; shift; fi
(( $# == 0 )) || { echo 'usage: scheduled-job.sh [--dry-run]' >&2; exit 2; }
run_id=${VERIFY_ALL_RUN_ID:-scheduled-$(date -u +%Y%m%dT%H%M%SZ)}
artifact_root=${VERIFY_ALL_ARTIFACT_ROOT:-$root/.agent-runs/verify}
summary="$artifact_root/$run_id/summary.json"
set +e
VERIFY_ALL_RUN_ID="$run_id" VERIFY_ALL_ARTIFACT_ROOT="$artifact_root" bash "$root/scripts/verify-all.sh"
verify_rc=$?
set -e
[[ -f $summary ]] || { echo "scheduled-job: verifier did not produce $summary" >&2; exit 2; }
python3 - "$summary" "$root" "$run_id" "$dry_run" <<'PY'
import json, os, subprocess, sys
from pathlib import Path
summary, root, run_id, dry_run = sys.argv[1:]
data = json.loads(Path(summary).read_text())
checks = data.get('checks')
if not isinstance(checks, list): raise SystemExit('scheduled-job: summary checks is not a list')
for check in checks:
    status = check.get('status')
    if not isinstance(status, int) or status == 0: continue
    name, log = check.get('name'), check.get('log')
    if not name or not log: raise SystemExit('scheduled-job: failed check lacks name or log')
    args = ['bash', str(Path(root) / 'scripts/file-defect.sh'), '--repo', os.environ.get('DEFECT_REPO', ''), '--run', run_id, '--feature', 'nightly verification', '--check', name, '--command', 'scripts/verify-all.sh', '--expected', 'exit 0', '--actual', f'exit {status}', '--evidence', log]
    if check.get('classification') == 'flaky': args += ['--classification', 'flaky']
    if dry_run == 'true': print('DRY RUN: ' + ' '.join(args))
    else: subprocess.run(args, check=True)
PY
adapter_rc=$?
(( adapter_rc == 0 )) || exit "$adapter_rc"
exit "$verify_rc"
