# Five minutes before you start

Read before your first action and after every compaction. Each rule is a mistake that really happened here; the
full story of each is in `docs/five-minutes-archive.md` (read it only if the rule is not enough).
Keep this file under 80 lines: add a rule only for a real incident, merge it into a neighbour if it fits, move the
story to the archive. Every line is re-read by every session.

1. **Read worker output before anything else.** An empty agent list does not mean nobody is working.
2. **Never state a number you did not measure on this commit.** Re-run the command or write `[?]`.
3. **Prove a check can fail.** Plant the error in a disposable worktree; the output must name what you planted
   (a crash or a typo also exits non-zero).
   - A control must break the promised behaviour with *identical inputs*: mutate the real guard, not the inputs.
   - Empty SQLmock expectations do not prove zero DB access; a repeated-write test is not an idempotency
     assertion (assert conflict keys and values); count values, not result lines (`grep -c` on aggregate output).
4. **Green gates are not acceptance.** Map each acceptance criterion to a test that exercises the real protocol:
   HTTP fixtures assert RequestURI and query, SQL tests assert conflict/update behaviour. Run golangci-lint
   sequentially (global lock).
5. **Merge pinned to the full SHA; then check the content landed** (`scripts/check-landed.sh <target> <branch>`).
   Never push to a merged PR's branch: `gh pr view <n> --json state -q .state | grep -qx OPEN || exit 1`.
6. **Verify the published tree, not the local checkout.** Inspect the staged tree before committing; compare the
   full published diff with the base (one PR deleted 146 files). A PR with no CI may simply conflict: check
   `mergeable`. Fetch the base right before syncing and keep every current coordinator record.
7. **Local is not hosted.** A CI negative control needs a hosted run that fails at the intended step. Smoke
   evidence comes from the final committed tree; run embedded interpreter code directly when stderr is
   suppressed; relocated shell controls must still resolve the production root.
8. **Documented config must match a real consumer.** Trace each env var to its loader and its use.
9. **Fixtures clean up after themselves.** After smoke, inspect containers *and* volumes; clean on every exit.
   A collision claim must match the complete uniqueness key.
10. **Cross-repo work:** start the worker in its own worktree (`--dir`), then check remote, branch and HEAD; a
    path does not choose the repo. Resume briefs are self-contained and readable from the worker's session root.
    Run helpers and `git worktree remove` from the owning repo (absolute paths). Create shared-repo worktrees
    one at a time before parallel dispatch.
11. **Moving issues keeps the contract, not only the title.** Transfer drops labels; compare destination bodies
    with `userContentEdits` history and audit every label dimension.
12. **Archive ignored evidence first.** `--delete-branch` can remove the worktree and its ignored logs; put
    acceptance artifacts under the root `.agent-runs/` before merging.

Underneath all of them: proving an adjacent thing is not proving the thing.
