#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CLIENT_SHA=3bedee0cff61b9f10a4fd591d8666460eba3f337
CLIENT_DIR=${CLIENT_DIR:-"$ROOT/.agent-runs/agent-ops-client"}

cd "$ROOT/api"
API_DOCS=(docs/docs.go docs/swagger.json docs/swagger.yaml)
api_before=$(mktemp -d)
trap 'rm -rf "$api_before"' EXIT
for path in "${API_DOCS[@]}"; do
  if [[ -e "$path" ]]; then
    mkdir -p "$api_before/$(dirname "$path")"
    cp -p "$path" "$api_before/$path"
    : > "$api_before/$path.present"
  fi
done
make swagger
api_drift=0
for path in "${API_DOCS[@]}"; do
  if [[ -e "$api_before/$path.present" ]]; then
    if [[ ! -f "$path" ]] || ! cmp -s "$api_before/$path" "$path"; then api_drift=1; fi
  elif [[ -e "$path" ]]; then
    api_drift=1
  fi
done
if (( api_drift )); then
  echo "API contract drift: regenerate and commit api/docs/{docs.go,swagger.json,swagger.yaml}" >&2
  for path in "${API_DOCS[@]}"; do
    if [[ ! -e "$api_before/$path.present" && -e "$path" ]]; then echo "  new: api/$path" >&2
    elif [[ -e "$api_before/$path.present" && ! -f "$path" ]]; then echo "  missing: api/$path" >&2
    elif [[ -e "$api_before/$path.present" ]] && ! cmp -s "$api_before/$path" "$path"; then echo "  changed: api/$path" >&2
    fi
  done
  exit 1
fi

if [[ ! -d "$CLIENT_DIR/.git" ]]; then
  mkdir -p "$(dirname "$CLIENT_DIR")"
  git clone https://github.com/0Hoag/agent-ops-client.git "$CLIENT_DIR"
fi
git -C "$CLIENT_DIR" fetch --quiet origin "$CLIENT_SHA"
if [[ "${CLIENT_CHECKOUT:-pinned}" == pinned ]]; then
  git -C "$CLIENT_DIR" checkout --quiet --detach "$CLIENT_SHA"
elif [[ "${CLIENT_CHECKOUT:-pinned}" != current ]]; then
  echo "CLIENT_CHECKOUT must be pinned or current" >&2
  exit 2
fi

cd "$CLIENT_DIR"
if [[ "${CLIENT_CHECKOUT:-pinned}" == pinned && -n "$(git status --porcelain)" ]]; then
  echo "Client fixture must start clean: $CLIENT_DIR" >&2
  exit 1
fi
CLIENT_ARTIFACTS=(src/shared/api/swagger.json src/shared/api/generated)
client_before=$(mktemp -d)
trap 'rm -rf "$api_before" "$client_before"' EXIT
for path in "${CLIENT_ARTIFACTS[@]}"; do
  if [[ -e "$path" ]]; then
    mkdir -p "$client_before/$(dirname "$path")"
    cp -Rp "$path" "$client_before/$path"
    : > "$client_before/$path.present"
  fi
done
cp "$ROOT/api/docs/swagger.json" src/shared/api/swagger.json
corepack pnpm@11.21.0 install --frozen-lockfile
corepack pnpm@11.21.0 generate:api
client_drift=0
for path in "${CLIENT_ARTIFACTS[@]}"; do
  if [[ -e "$client_before/$path.present" ]]; then
    if [[ ! -e "$path" ]] || ! diff -qr "$client_before/$path" "$path" >/dev/null; then client_drift=1; fi
  elif [[ -e "$path" ]]; then
    client_drift=1
  fi
done
if (( client_drift )); then
  echo "Client contract drift: update the committed API snapshot and generated client at $CLIENT_SHA" >&2
  for path in "${CLIENT_ARTIFACTS[@]}"; do
    if [[ ! -e "$client_before/$path.present" && -e "$path" ]]; then echo "  new: $path" >&2
    elif [[ -e "$client_before/$path.present" && ! -e "$path" ]]; then echo "  missing: $path" >&2
    elif [[ -e "$client_before/$path.present" ]] && ! diff -qr "$client_before/$path" "$path" >/dev/null; then echo "  changed: $path" >&2
    fi
  done
  exit 1
fi
