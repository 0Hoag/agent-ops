# API contract drift verification

Pull requests regenerate the pinned OpenAPI artifacts and regenerate the client from this PR's schema
against the client repository pinned in `scripts/check-api-contract.sh`. Run
`bash scripts/check-api-contract.sh` locally; it creates `.agent-runs` if needed and clones the pinned
client into `.agent-runs/agent-ops-client` unless `CLIENT_DIR` points elsewhere. Generation runs only
in disposable trees: the API is created from the current commit and the client from the pinned full
SHA. The check compares the complete declared artifact sets, including changed, missing, and newly
created artifacts, and never checks out, resets, or otherwise rewrites caller-owned files. A pre-existing
caller artifact edit is therefore preserved even when it causes the check to report drift.

Run `bash scripts/contract-drift-fixtures.sh` from any directory. The script invokes the real
`scripts/check-api-contract.sh` for the baseline and every mutation, using an isolated client checkout
at pinned commit `3bedee0cff61b9f10a4fd591d8666460eba3f337`. It covers a valid handler annotation that
changes generated Swagger, API missing/new outputs, and client changed/missing/new declared outputs.

The fixtures are owned by `.agent-runs/contract-controls`; the script restores the handler source and
API docs and removes that fixture directory on exit. It creates tracked and untracked caller edits in
its disposable client clone, snapshots those plus contract artifacts before each actual gate run, and
requires unchanged hashes afterward. Do not point `CLIENT_DIR` at a user-owned checkout: the script
creates its own pinned client clone, and the production gate only archives the pinned commit for
generation. `CONTRACT_FIXTURE=api-missing|client-missing` removes a successfully generated artifact
only inside the gate's disposable copy, after the real generator completes; it does not bypass gate
comparison or synthesis.

Expected successful output includes `PASS baseline: exit 0`, each negative case `PASS ...: exit 1`
with its expected artifact diagnostic, and `PASS restoration: caller/API hashes match`.
The API/client comparison logic treats missing, new, and changed paths distinctly. Keep the client
pin, `corepack pnpm@11.21.0`, and generator templates unchanged unless a separately authorized issue
requires updating the generated contract.

For an intentional contract change, regenerate and commit `api/docs/` in this repository, then update
the client repository's `src/shared/api/swagger.json` and generated output using its existing
`pnpm generate:api` (preserving `scripts/api-templates/`). Merge the client change first, then update
`CLIENT_SHA` to its full commit SHA here. Do not update the pin to a branch or tag.
