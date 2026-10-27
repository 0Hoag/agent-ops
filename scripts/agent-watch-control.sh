#!/usr/bin/env bash
set -Eeuo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CONTROL=$(mktemp -d)
PROJECT="agent-watch-control-$(id -u)-$$"
API_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
PG_PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
cleanup() { local status=$?; if [[ -d "$CONTROL" ]]; then python3 -c 'import pathlib,sys; p=pathlib.Path(sys.argv[1]); [x.unlink() if x.is_file() or x.is_symlink() else x.rmdir() for x in sorted(p.rglob("*"), key=lambda x: len(x.parts), reverse=True)]; p.rmdir()' "$CONTROL"; fi; exit "$status"; }
trap cleanup EXIT
cp -R "$ROOT/api" "$CONTROL/api"
cp "$ROOT/docker-compose.yml" "$CONTROL/docker-compose.yml"
python3 - "$CONTROL/docker-compose.yml" "$PROJECT" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1])
s = p.read_text()
s = s.replace('agent_ops_postgres:', f'{sys.argv[2]}_postgres:')
p.write_text(s)
PY
python3 - "$CONTROL/api/cmd/agentops/main.go" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1])
s = p.read_text()
old = '"session_id": session.ID'
new = '"session_id": session.ID + "-changed"'
assert s.count(old) == 1, 'control: expected exactly one submitted session identity'
p.write_text(s.replace(old, new))
PY
printf 'watch-control: compiling disposable session identity regression\n'
(cd "$CONTROL/api" && go build -o "$CONTROL/agentops" ./cmd/agentops)
printf 'watch-control: running identical acceptance inputs; expected exactly-one SQL assertion failure\n'
set +e
AGENT_WATCH_BIN_OVERRIDE="$CONTROL/agentops" AGENT_WATCH_PROJECT="$PROJECT" AGENT_WATCH_API_PORT="$API_PORT" AGENT_WATCH_PG_PORT="$PG_PORT" AGENT_WATCH_REPOSITORY_ID="a9000000-0000-4000-8000-000000000002" AGENT_WATCH_SESSION_ID=synthetic-session-changed AGENT_WATCH_ARTIFACT_DIR="${AGENT_WATCH_ARTIFACT_DIR:?set artifact dir}" bash "$ROOT/scripts/agent-watch-acceptance.sh" >"$CONTROL/control.log" 2>&1
status=$?
set -e
cat "$CONTROL/control.log"
if (( status == 0 )); then printf 'watch-control: FAIL mutation unexpectedly passed\n' >&2; exit 1; fi
if ! python3 - "$CONTROL/control.log" <<'PY'
from pathlib import Path
import sys
s = Path(sys.argv[1]).read_text()
assert 'completed expected 1|32|32|6|6|11|11|8|8|0|0 got 2|' in s, 'control: missing intended exactly-one cumulative SQL diagnostic'
print('watch-control: PASS intended duplicate cumulative SQL diagnostic observed')
PY
then exit 1; fi
