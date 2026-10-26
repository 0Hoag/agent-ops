#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CLIENT_SHA=1d692bfdf33855a1ef566a2fceff69d2c9f8239f
CLIENT_DIR=${CLIENT_DIR:-"$ROOT/.agent-runs/agent-ops-client"}
CLIENT_REPO=https://github.com/0Hoag/agent-ops-client.git
case "${CONTRACT_FIXTURE:-}" in
  ""|handler|api-missing|api-new|client-missing|client-new|client-changed) ;;
  *) echo "Unknown CONTRACT_FIXTURE: $CONTRACT_FIXTURE" >&2; exit 2 ;;
esac

mkdir -p "$ROOT/.agent-runs"
cd "$ROOT/api"
API_DOCS=(docs/docs.go docs/swagger.json docs/swagger.yaml)
tmp=$(mktemp -d "$ROOT/.agent-runs/contract.XXXXXX")
restore_controller=0
restore_swagger=0
cleanup() {
  if (( restore_controller )); then cp "$tmp/fixture-controller.go" "$ROOT/api/modules/insight/controller.go"; fi
  if (( restore_swagger )); then cp "$tmp/fixture-swagger.yaml" "$ROOT/api/docs/swagger.yaml"; fi
  rm -rf "$tmp"
}
trap cleanup EXIT
mkdir -p "$tmp/api"
if [[ "${CONTRACT_FIXTURE:-}" == handler ]]; then
  cp "$ROOT/api/modules/insight/controller.go" "$tmp/fixture-controller.go"
  restore_controller=1
  python3 - "$ROOT/api/modules/insight/controller.go" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
s = p.read_text()
needle = '// @Failure 500 {object} response.ErrorResponse'
if needle not in s:
    raise SystemExit('handler fixture anchor missing')
p.write_text(s.replace(needle, needle + '\n// @Failure 418 {object} response.ErrorResponse', 1))
PY
fi
cp -R "$ROOT/api/." "$tmp/api/"
for path in "${API_DOCS[@]}"; do rm -rf "$tmp/api/$path"; done
cd "$tmp/api"
make swagger
if [[ "${CONTRACT_FIXTURE:-}" == api-missing ]]; then
  rm "$tmp/api/docs/swagger.yaml"
fi
if [[ "${CONTRACT_FIXTURE:-}" == api-new ]]; then
  cp "$ROOT/api/docs/swagger.yaml" "$tmp/fixture-swagger.yaml"
  restore_swagger=1
  rm "$ROOT/api/docs/swagger.yaml"
fi
api_drift=0
compare_paths() {
  local baseline=$1 generated=$2 label=$3 drift=0 path expected found
  shift 3
  local artifacts=("$@")
  for path in "${artifacts[@]}"; do
    expected=0; found=0
    [[ ! -e "$baseline/$path" ]] || expected=1
    [[ ! -e "$generated/$path" ]] || found=1
    if (( expected && ! found )); then
      echo "  missing: $label/$path" >&2; drift=1
    elif (( ! expected && found )); then
      echo "  new: $label/$path" >&2; drift=1
    elif (( expected && found )) && ! diff -qr "$baseline/$path" "$generated/$path" >/dev/null 2>&1; then
      echo "  changed: $label/$path" >&2; drift=1
    fi
  done
  return "$drift"
}
if ! compare_paths "$ROOT/api" "$tmp/api" api "${API_DOCS[@]}"; then api_drift=1; fi
if (( api_drift )); then
  echo "API contract drift: regenerate and commit api/docs/{docs.go,swagger.json,swagger.yaml}" >&2
  exit 1
fi
if [[ ! -d "$CLIENT_DIR/.git" ]]; then
  mkdir -p "$(dirname "$CLIENT_DIR")"
  git clone "$CLIENT_REPO" "$CLIENT_DIR"
fi
git -C "$CLIENT_DIR" fetch --quiet origin "$CLIENT_SHA"
if [[ "${CLIENT_CHECKOUT:-pinned}" != pinned ]]; then
  echo "CLIENT_CHECKOUT must be pinned or current" >&2
  exit 2
fi

client_work="$tmp/client"
mkdir -p "$client_work"
git -C "$CLIENT_DIR" archive "$CLIENT_SHA" | tar -x -C "$client_work"
client_baseline="$tmp/client-baseline"
mkdir -p "$client_baseline"
git -C "$CLIENT_DIR" archive "$CLIENT_SHA" | tar -x -C "$client_baseline"
if [[ "${CONTRACT_FIXTURE:-}" == client-new ]]; then
  rm -rf "$client_baseline/src/shared/api/generated"
fi
cd "$client_work"
CLIENT_ARTIFACTS=(src/shared/api/swagger.json src/shared/api/generated)
cp "$ROOT/api/docs/swagger.json" src/shared/api/swagger.json
corepack pnpm@11.21.0 install --frozen-lockfile
corepack pnpm@11.21.0 generate:api
if [[ "${CONTRACT_FIXTURE:-}" == client-missing ]]; then
  rm -rf "$client_work/src/shared/api/generated"
fi
if [[ "${CONTRACT_FIXTURE:-}" == client-changed ]]; then
  printf '\nfixture change\n' >> "$client_work/src/shared/api/generated/index.ts"
fi
client_drift=0
if ! compare_paths "$client_baseline" "$client_work" client "${CLIENT_ARTIFACTS[@]}"; then client_drift=1; fi
if (( client_drift )); then
  echo "Client contract drift: update the committed API snapshot and generated client at $CLIENT_SHA" >&2
  exit 1
fi
