#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CLIENT_SHA=3bedee0cff61b9f10a4fd591d8666460eba3f337
CLIENT_DIR=${CLIENT_DIR:-"$ROOT/.agent-runs/agent-ops-client"}

cd "$ROOT/api"
restore_api_docs() {
  git -C "$ROOT" restore --worktree -- api/docs/docs.go api/docs/swagger.json api/docs/swagger.yaml
}
trap restore_api_docs EXIT
make swagger
if ! git diff --quiet -- docs/docs.go docs/swagger.json docs/swagger.yaml; then
  echo "API contract drift: regenerate and commit api/docs/{docs.go,swagger.json,swagger.yaml}" >&2
  git --no-pager diff --stat -- docs/docs.go docs/swagger.json docs/swagger.yaml >&2
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
cp "$ROOT/api/docs/swagger.json" src/shared/api/swagger.json
corepack pnpm@11.21.0 install --frozen-lockfile
corepack pnpm@11.21.0 generate:api
if ! git diff --quiet -- src/shared/api/swagger.json src/shared/api/generated; then
  echo "Client contract drift: update the committed API snapshot and generated client at $CLIENT_SHA" >&2
  git --no-pager diff --stat -- src/shared/api/swagger.json src/shared/api/generated >&2
  exit 1
fi
