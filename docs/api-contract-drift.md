# API contract drift

Pull requests regenerate the pinned OpenAPI artifacts and regenerate the client from this PR's schema against the client repository pinned in `scripts/check-api-contract.sh`. Run `bash scripts/check-api-contract.sh` locally; it clones the pinned client into `.agent-runs/agent-ops-client` unless `CLIENT_DIR` points elsewhere.

For an intentional contract change, regenerate and commit `api/docs/` in this repository, then update the client repository's `src/shared/api/swagger.json` and generated output using its existing `pnpm generate:api` (preserving `scripts/api-templates/`). Merge the client change first, then update `CLIENT_SHA` to its full commit SHA here. Do not update the pin to a branch or tag.
