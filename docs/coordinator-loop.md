# Coordinator loop (the Autopilot pass)

Coordinator only. Workers do not read this file; `AGENTS.md` points here. One pass of `scripts/autopilot.sh` follows it.

The loop is driven by **`scripts/autopilot.sh`** (the owner starts it once). Each call is a fresh, short
session that does **one pass** and exits; the shell sleeps between passes (no tokens spent waiting).
The **status board issue**, the issues and the PRs are the memory between passes: write there, not in chat.
Run in a terminal: `scripts/autopilot.sh` (every 10 min) · stop: `touch .agent-runs/STOP`.

The shell automatically waits for worker results or GitHub changes, including issue title/body/label/update
changes and comments. A failed GitHub fingerprint is an `API ERROR` (exit 2), never treated as unchanged. With no workers, no in-progress
issues, no observed pending PR checks/development CI, and unchanged GitHub it emits `STALLED` (exit 2) rather than polling forever; pending/running work is not
reported as completed. Recovery is coordinator-owned: assess the stable repo+issue/unit identity and authorize at
most one narrow recovery; record the authorization externally before dispatch. Renaming a wave does not reset the
run budget. The shell does not dispatch recovery, approve budgets, or infer completion.

For the owner-authorized recovery, review the issue/unit and write a narrow brief with stop criteria first. Then
dispatch using:

```bash
bash scripts/spawn.sh --recovery 0Hoag/agent-ops 408 unit-id /absolute/recovery-408.log scripts/run-worker.sh W408-r2 fix/408-r2 briefs/408-recovery.md
```

The wrapper atomically reserves one attempt per repository+issue under the canonical shared Git directory
(`$(git rev-parse --git-common-dir)/recovery/`); changing worktree, CWD or unit cannot reset it. No state-directory
override exists. Authorization and the two implementation attempts are coordinator accounting; the launcher enforces
the single recovery reservation, written before executing the command, retained even on failure. Existing callers remain supported:
`scripts/spawn.sh <log> <command> [args...]`. A reservation is durable even if its command fails, so validate the
launcher/environment before invoking the guarded form. The guarded wrapper checks the launcher, Python, Git, gh and jq
before reservation, plus Codex/OpenCode for the corresponding worker launcher. Direct worker preflight requires only its
own harness (Codex does not require OpenCode). Optional `AUTOPILOT_REQUIRED_TOOLS` is a space-separated list of executable
names checked by `autopilot_preflight`; `AUTOPILOT_PREFLIGHT_PROBE` is an executable absolute path run synchronously by
that same helper. Declare environment requirements there (for example a browser probe that actually launches the required
browser and exits nonzero if dependencies are missing). Tool/probe failures do not consume recovery. These variables are
inherited by worker preflight; neither edits a manifest or global configuration. Empty/malformed final output stops safely. Only
`AUTOPILOT: CONTINUE` or `AUTOPILOT: STOP ...` first lines are valid. Issue comments are included in the fingerprint
along with labels/body/title. Stalled means no local worker/stale in-progress issue was found and the observed
GitHub fingerprint was unchanged; it does not assert that all external/CI work is blocked. Fingerprint/status API
errors stop with an explicit diagnostic. Live workers wait up to `INTERVAL` (`3 * INTERVAL` at `CAP`) before a heartbeat
pass; observed queued/running PR checks or latest development CI wake a pass after `INTERVAL` rather than declaring
idle completion. These are bounded waits, not a claim that CI will finish. Quota errors with no report retain the
15-minute retry; malformed JSON or a nonempty malformed report still fails safely.

One pass:

1. **Sync.** `git pull`; `gh pr list`; open issues; for each running worker read only
   `tail -20 .agent-runs/<wave>.log` and `.agent-runs/<wave>.md` if present (never the whole `.jsonl`).
   Read the status board issue body.
2. **Plan: many small issues, all up front.** Break the whole ROADMAP into fine-grained issues before
   dispatching: one issue per concrete deliverable (one endpoint, one table + repository, one provider
   event, one page, one component, one script), each with "Done when" as a command or number. Expect
   dozens, not a handful. Labels `status:*`, `priority:*`, `type:*`, `wave:*`, `area:api|client|ci|docs`.
   A wave = several issues in the same area, given to one worker; its PR closes all of them.
   Mark dependencies (`status:blocked` + `Status:` comment).
   **Real defects discovered while running become a new issue**, never an inline fix: a bug, a test that
   cannot fail, a security or data-loss risk, an acceptance criterion a merged PR missed. Label it
   `found-by:agent`, put it in a later wave. "Would be nice to also have" is **not** an issue: skip it.
3. **Finish open PRs: spot-check, do not redo the work.**
   **`scripts/autopilot.sh` already squash-merges, without tokens, every non-critical PR whose CI is fully green** (the
   per-PR `PR gate` job enforces `Closes #n`, a test for every product change, and "tests fail without the change"; see
   `scripts/ci/pr-gate.sh`). Do not review or merge those. Your job in this step is only PRs the script left:
   red CI (read the failing job log tail once, send back once with that error), merge conflicts, no `Closes #n`, and
   **CRITICAL** PRs (`api/main.go`, `api/pkg/{auth,middleware,database,crypto}/`, `.github/workflows/`, `scripts/ci/`,
   autopilot scripts, destructive migrations per `scripts/ci/classify-migration.sh`): review the diff, then merge.
   The rules below apply to those. For each PR whose worker has finished:
   - The **PR body must carry the evidence**: gate commands with output, smoke result, the failing-control
     proof for new tests. Missing evidence = send back (cheap), do not produce it yourself.
   - CI green on the exact head, `Closes #n`, no out-of-scope files (`gh pr diff --name-only`).
   - **Re-run at most 2 checks yourself**, the ones most likely to be false (e.g. one test, one curl).
     Do not rebuild full acceptance environments in the coordinator pass; that is the worker's job.
   - Then merge (squash, pinned SHA), confirm the content is on `main`, remove its worktree.
   - **Hygiene is scripted, you only read it.** Before each pass `autopilot.sh` runs `scripts/worktrees.sh --prune` and `scripts/branches.sh --delete` (deletes
     only branches whose tip is exactly a merged PR's head; never `main`/`reviews/development`/PR bases) and
     `scripts/audit.sh` (missing/double status, no priority, blocked without a `Status:` note, closed issue still
     labelled, PR naming no issue). Read `.agent-runs/coordinator/branches.txt` and `audit.txt`; empty = healthy.
     `LATE` = commits pushed after a merged PR (lost work: open a new PR from that branch), `NO-PR` = unpublished work
     (open a draft PR). Fix every audit line this pass; it is cheaper than the confusion it causes later.
   - **At most 1 send-back per PR** (owner 29/10: too many send-backs, little delivered). On the 2nd review, if CI
     and gates are green and the feature works, merge. Missing evidence is **not** a reason to send back twice and
     **not** a reason for an evidence-only follow-up PR; file a `found-by:agent` issue only for a real defect (wrong
     behaviour, a test that cannot fail). Send back at the 1st review only for: red CI, out-of-scope files, no
     `Closes #n`, or a failing check you re-ran yourself.
4. **Dispatch.** If fewer than **2 workers** are running (`pgrep -f run-worker`; cap 2, owner 29/10: 3 heated the machine and delivered little), start the next unblocked
   sub-waves (different directories only). English brief in `briefs/<wave>.md` with exact paths, line numbers
   and commands; every brief says "commit and push after every finished issue". Every brief also says: run `make ci` in `api/` (or the client gate) once before opening the PR, not after every edit. Start **detached** so it survives this pass:
   `scripts/spawn.sh .agent-runs/<wave>.log scripts/run-worker*.sh <wave> <branch> briefs/<wave>.md`
   (plain `nohup … &` is killed when the pass ends). Harness: **OpenCode for every `area:client` wave**
   (Codex's sandbox cannot write the client worktree's git dir, run Chromium or `rm -f`; owner 26/10, W7r r7-r9 never
   pushed); Codex only for backend waves. When restarting a client wave whose worktree has uncommitted work, the
   RESUME block names those files so the worker verifies and commits them instead of redoing them.
   **One unit per worker, shared block first, worktree removed.** A brief covers one issue/wave; when done the worker
   reports and stops (measured by the owner's team: per-item sub-agents averaged 0.42, one agent given a whole list
   5.81). In a batch the shared rules block is identical to the byte and first (prompt-cache prefix); start one worker,
   then the rest. The worker removes its worktree once the PR is open (`scripts/worktrees.sh --remove <path>`, refuses
   dirty or unpushed); the pass runs `scripts/worktrees.sh --prune`. After a merge run
   `scripts/check-landed.sh reviews/development <branch>` (silent = landed). Other roles: `docs/briefs/{reviewer,architect,verifier}.md`.
   **Model per task:** mechanical runs (gates, summaries, list edits) -> the cheap worker model; code that can lose data,
   conflict resolution, live verification, reviewer/architect briefs -> the coordinator model. Never accept a cheap
   model's report as evidence: re-run its check.
   **Brief = deliverable first.** Each brief states the feature to ship and its "Done when" command, and asks for
   evidence in the PR body in one fixed shape: gate output, one smoke or test result, one failing control. Prefer
   waves that ship features (`feat`) over waves that only harden smoke/CI; never open a wave whose only goal is more
   acceptance evidence for an already merged PR.
   **Run budget (owner 29/10: 65% of worker tokens went into 2nd+ runs of the same wave):** a wave gets at most
   **2 worker runs** (first + one RESUME). After that: CI green -> merge (normal checks); if acceptance is still
   unmet, open a `found-by:agent` issue with exactly the unmet part (`Split-of: #<original>` in its body) and move on
   (owner 06/11: blocking green PRs on the budget stalled the loop). At most 2 splits per original feature; on the
   3rd, or with CI red after the CI-fix run, mark the issue `status:blocked`, comment why, and move to the next wave. Never start a 3rd run on the same wave
   (`scripts/spawn.sh` refuses a `-r3` wave name; put the leftover work in a new issue instead).
   **Red PRs are work, not a wait state.** Every pass, for each open PR with a failed check and no worker on its branch
   (`pgrep -f <branch>`), dispatch ONE fix worker on that branch unless the PR is CRITICAL. A CI-fix run is **exempt
   from the run budget** (owner 30/10: the budget deadlocked three red PRs, nothing ran): brief it narrowly, no new
   scope: "rebase on origin/reviews/development, make only the failing check green, push", one fix run per PR head SHA.
   Likewise act, do not just report: PR with a conflict -> fix worker rebases it; PR missing `Closes #n` -> add it with
   `gh pr edit --body`. Do not leave `status:blocked` issues idle behind a red PR.
   **Contract drift (`Client contract drift` red, "update the committed API snapshot"):** `scripts/check-api-contract.sh`
   pins `CLIENT_SHA` to a commit of `agent-ops-client`. An API PR that changes routes needs the matching client
   contract PR merged first, then a pin bump in the API PR to that client `reviews/development` SHA (the SHA that
   contains this PR's contract and no later one). Merge API PRs one at a time; the next one rebases and bumps again.
   **Correctness without evidence churn (owner 29/10: output must be right, not just green).** Every PR that adds or
   changes product code must pass these cheap, deterministic checks; they replace live-smoke asks on ordinary PRs:
   1. Each changed product file has a test in the same PR (`gh pr diff --name-only`); no test = send back.
   2. **Fails before, passes after:** the coordinator checks out the PR's new test files onto `origin/main` in a
      scratch worktree and runs that one test command: it must fail there and pass on the PR head. This is one of
      the 2 re-run checks and is mandatory for `feat`/`fix` PRs.
   3. CI green on the exact head, including contract-drift; the diff stays inside the issue's paths.
   Live smoke or browser evidence is required only in the smoke/e2e waves (WS-d, W8e, W16) and for client pages.
   **Light machine:** workers run targeted checks while developing (one package: `go test ./internal/x/...`, one
   vitest file) and the full gate once before the PR; CI is the authoritative full gate. Browser, `docker compose`
   and full builds go through `scripts/heavy.sh <command>` (one at a time, low priority). Put this in every brief.
   A worker with no log change for 20 min: read its log tail, fix the brief, restart it.
   **Usage limit** in a worker log (usage/rate limit, quota, 429, "try again at"): not an owner question;
   restart that worker on the same branch in a later pass after the reset time.
   **Restart brief = old brief + a RESUME block** with five headings, DONE (merged/verified, how), IN FLIGHT (branch commits, uncommitted files, open PRs), NEXT (ordered, each with its command), BLOCKED ON, NOT COVERED built cheaply from `git log origin/main..<branch>`,
   `git status` in its worktree and the log tail: commits already on the branch, issues done, issues left
   (with files), last failing gate. Always a fresh worker session: the old one's cache has expired, so
   resuming it would resend its whole context.
5. **Record, without commits.** Current state lives in **one pinned GitHub issue labelled `status-board`**
   (create and `gh issue pin` it if missing; seed it from `STATUS.md`, then reduce `STATUS.md` to a one-line
   pointer). Body = wave table, cost table, 3-line "Now"; under 80 lines; a page, not a log. Update it with
   `gh issue edit <n> --body-file <file>` when something changed. **Never commit status updates.** Pass details
   go in the pass report (`.agent-runs/coordinator/`) and PR/issue comments.
   New agent mistakes go to `FIVE-MINUTES.md`, committed only together with a PR or at most once a day.
   Then run `node scripts/project-sync.mjs` (mirrors labels into the GitHub Project "Agent Ops": kanban,
   roadmap, one epic per wave with sub-issues; idempotent, no tokens). Every new issue needs `wave:*`,
   `area:*`, `priority:*`, `status:*` labels or it lands in the Project without them.
6. **End the pass** with a report of at most 10 lines. **First line** is one of:
   `AUTOPILOT: CONTINUE` (normal) · `AUTOPILOT: STOP roadmap done` · `AUTOPILOT: STOP owner decision: <question>`
   (only for ROADMAP "Câu hỏi mở" or the "ask the owner" list above). Never sleep or wait inside a pass.
