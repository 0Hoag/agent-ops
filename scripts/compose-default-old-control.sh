#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
control_log=$(mktemp)
cleanup() {
  rm -rf "$tmp"
  rm -f "$control_log"
}
trap cleanup EXIT INT TERM

# Keep the historical Compose file's relative build context rooted at the real repo.
git -C "$ROOT" show 7c0ab5f5b50d884ba322e01a307b71ab545f8f38:docker-compose.yml >"$tmp/docker-compose.yml"
cat >"$tmp/docker-compose.yaml" <<EOF
services:
  api:
    build:
      context: $ROOT
      dockerfile: api/Dockerfile
    environment:
      SCM_REPOSITORIES_JSON: \\${SCM_REPOSITORIES_JSON:-}
EOF
rm -f "$tmp/docker-compose.yaml"
if COMPOSE_FILE="$tmp/docker-compose.yml" EXPECT_OLD_DEFAULT_CONTROL=1 "$ROOT/scripts/compose-default-acceptance.sh" 2>"$control_log"; then
  python3 - "$control_log" <<'PY'
import pathlib, sys
print(pathlib.Path(sys.argv[1]).read_text(), end="")
PY
else
  status=$?
  python3 - "$control_log" <<'PY'
import pathlib, sys
print(pathlib.Path(sys.argv[1]).read_text(), end="")
PY
  exit "$status"
fi
