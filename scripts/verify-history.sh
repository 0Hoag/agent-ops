#!/usr/bin/env bash
set -Eeuo pipefail

summary=${1:?usage: verify-history.sh SUMMARY}
history=${VERIFY_HISTORY_FILE:-$(dirname "$summary")/../../../verify-history.json}
python3 - "$summary" "$history" <<'PY'
import json, os, sys
from pathlib import Path

summary_path, history_path = map(Path, sys.argv[1:])
data = json.loads(summary_path.read_text())
checks = data.get("checks")
if not isinstance(checks, list):
    raise SystemExit("verify-history: summary checks is not a list")
source = data.get("source_sha")
if not source:
    raise SystemExit("verify-history: summary source_sha is missing")

if history_path.exists():
    history = json.loads(history_path.read_text())
else:
    history = {}
if not isinstance(history, dict):
    raise SystemExit("verify-history: history is not an object")

for check in checks:
    name = check.get("name")
    status = check.get("status")
    if not isinstance(name, str) or not isinstance(status, int):
        raise SystemExit("verify-history: check lacks name or integer status")
    entries = history.get(name, [])
    if not isinstance(entries, list):
        raise SystemExit(f"verify-history: history for {name} is not a list")
    same_source = [entry for entry in entries if entry.get("source_sha") == source]
    flaky = bool(same_source and same_source[-1].get("status") != status)
    check["flaky"] = flaky
    check["classification"] = "flaky" if flaky else ("failure" if status else "pass")
    history[name] = (entries + [{"source_sha": source, "status": status}])[-30:]

history_path.parent.mkdir(parents=True, exist_ok=True)
tmp = history_path.with_name(history_path.name + ".tmp")
tmp.write_text(json.dumps(history, indent=2) + "\n")
os.replace(tmp, history_path)
summary_path.write_text(json.dumps(data, indent=2) + "\n")
PY
