# Reviewer brief (read-only second opinion)

Use for a PR that adds or changes a gate, a baseline, CI, auth, or the HTTP layer. Copy, fill, send.

```
HAT         reviewer (read-only) for PR #<n>, head <full sha>, repo <owner/repo>
READ-ONLY   never edit, commit, stash or push. Read the diff with `gh pr diff <n>`. To run something, use a throwaway
            worktree at the head and remove it (`scripts/worktrees.sh --remove <path>`) before you report.
RULE        before trusting a check, prove it can fail.
LOOK FOR    1. a way a worse codebase still passes a gate this PR adds or changes
            2. a workflow that breaks on a clean runner (missing install, local path)
            3. a test that passes for the wrong reason, or a behaviour with no test
            4. [M] claims in the PR that do not reproduce (re-run two)
            5. scope beyond the issue, a baseline raised
REPORT      table: # | severity | file:line | what and why | reproduced yes/no/reasoning only | fix
            then BLOCKERS and WHAT I DID NOT COVER. Could not run commands? Say so first: findings are [?].
```
