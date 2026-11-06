#!/usr/bin/env bash
set -Eeuo pipefail
summary=${1:?summary JSON required}; persisted=${2:?persisted count required}; successful=${3:?successful response count required}
python3 - "$summary" "$persisted" "$successful" <<'PY'
import json,sys
s=json.load(open(sys.argv[1])); m=s['metrics']; persisted=int(sys.argv[2]); successful=int(sys.argv[3])
dropped=int(m['dropped_iterations']['count']); completed=int(m['iterations_completed']['count']); failed=float(m['http_req_failed']['rate'])
if persisted != successful: raise SystemExit(f'persisted delivery count mismatch: persisted={persisted} successful={successful}')
if dropped != 0: raise SystemExit(f'dropped iterations: {dropped}')
if completed != successful: raise SystemExit(f'completed/success mismatch: completed={completed} successful={successful}')
if failed != 0: raise SystemExit(f'non-2xx responses: rate={failed}')
print(f'reconciled persisted={persisted} successful={successful} completed={completed} dropped={dropped}')
PY
