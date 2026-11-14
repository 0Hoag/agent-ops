#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
verify_dir=${VERIFY_ALL_CHECK_DIR:-"$root/scripts/verify"}
stamp=${VERIFY_ALL_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}
artifact_dir="${VERIFY_ALL_ARTIFACT_ROOT:-$root/.agent-runs/verify}/$stamp"
mkdir -p "$artifact_dir/logs"
source_sha=$(git -C "$root" rev-parse HEAD 2>/dev/null || echo unknown)
summary="$artifact_dir/summary.json"; table="$artifact_dir/table.txt"
python3 - "$summary" "$source_sha" "$verify_dir" <<'PY'
import json,sys
from pathlib import Path
Path(sys.argv[1]).write_text(json.dumps({'source_sha':sys.argv[2],'check_directory':sys.argv[3],'checks':[]},indent=2)+'\n')
PY
printf 'VERIFY ALL source=%s\n' "$source_sha" | tee "$table"
printf '%-36s %-6s %s\n' check status log | tee -a "$table"
[[ -d "$verify_dir" ]] || { echo "setup error: check directory does not exist: $verify_dir" >&2; exit 2; }
checks=()
while IFS= read -r check_path; do checks+=("$check_path"); done < <(find "$verify_dir" -maxdepth 1 -type f -name '*.sh' -print | sort)
((${#checks[@]})) || { echo "setup error: no top-level verifier checks found in $verify_dir" >&2; exit 2; }
overall=0
for check in "${checks[@]}"; do
  name=${check##*/}; name=${name%.sh}; log_path="$artifact_dir/logs/$name.log"; status=0
  if [[ "$check" == "$root/scripts/verify-all.sh" ]]; then printf 'setup error: refusing self-invocation: %s\n' "$check" >"$log_path"; status=2
  elif [[ ! -x "$check" ]]; then printf 'setup error: verifier is not executable: %s\n' "$check" >"$log_path"; status=2
  else "$check" >"$log_path" 2>&1 || status=$?; fi
  ((status == 0)) || overall=1
  python3 - "$summary" "$name" "$status" "$log_path" <<'PY'
import json,sys
from pathlib import Path
p=Path(sys.argv[1]); d=json.loads(p.read_text()); s=int(sys.argv[3]); d['checks'].append({'name':sys.argv[2],'status':s,'result':'pass' if s==0 else 'fail','log':sys.argv[4]}); p.write_text(json.dumps(d,indent=2)+'\n')
PY
  printf '%-36s %-6s %s\n' "$name" "$status" "$log_path" | tee -a "$table"
done
python3 - "$summary" "$table" <<'PY'
import json,sys
from pathlib import Path
p=Path(sys.argv[1]); d=json.loads(p.read_text()); d['table']=sys.argv[2]; p.write_text(json.dumps(d,indent=2)+'\n')
PY
printf 'summary=%s\n' "$summary"
exit "$overall"
