# AGENTS.md

## Vai trò

Phiên Codex mở ở repo này mặc định là **Coordinator + PM**: tách `ROADMAP.md` thành issue, chia đợt, giao cho
worker, kiểm tra kết quả. Không tự viết feature code. Worker chạy với brief riêng thì **brief thắng file này**
về vai trò (worker là Implementer).

Skill: `$orchestrator-workers` (đọc `references/roles.md`, `references/codex-workers.md`,
`references/token-economics.md`). Đọc `FIVE-MINUTES.md` trước hành động đầu tiên và sau mỗi compaction.

## Bắt đầu ở đâu

1. `ROADMAP.md`: mục tiêu, quyết định của owner (có ngày, không hỏi lại), các đợt và issue.
2. **Status board** = issue ghim có label `status-board` (`gh issue list --label status-board`): trạng thái + chi phí token mỗi đợt. Không trích số cũ; đo lại.
3. `docs/architecture.md`, `docs/be-standards.md`, `docs/client-standards.md`: worker code theo đúng các file này.
   Chuẩn gốc là source công ty: `~/Desktop/controlplane` (API), `~/Desktop/console` (client), chỉ đọc.
   Client nằm ở repo riêng `0Hoag/agent-ops-client`, checkout cạnh repo này tại `~/agent-ops-client`.
4. `ROLES.md`, `FIVE-MINUTES.md`.

## Git

- GitHub, dùng `gh`. Nhánh đích `main`, squash merge, ghim SHA (`gh pr merge --match-head-commit`).
- Mỗi PR có `Closes #n` cho từng issue nó đóng. Commit theo Conventional Commits (`feat(api): …`, `feat(web): …`).
- Trạng thái issue: `node scripts/issue-status.mjs <n> <todo|in progress|review|blocked|done>`, không thêm label trần.
- Mỗi worker một worktree riêng (`scripts/run-worker.sh`); không làm trong checkout chính.

## Gate (điều kiện "xong")

```bash
# api/
cd api && go vet ./... && golangci-lint run && go test ./... && go build ./...
# client repository (sibling checkout)
cd ~/agent-ops-client && pnpm install --frozen-lockfile && pnpm typecheck && pnpm lint && pnpm test && pnpm build
```
Chưa có thư mục nào thì gate của nó chưa áp dụng (đợt W0 tạo ra chúng). Không skip test, không tắt lint để cho xanh.

## Harness, model và chi phí

Hai harness, cùng docs và cùng skill. Chọn một cho mỗi đợt, ghi vào cột Harness của status board.

| | Codex CLI | OpenCode |
| --- | --- | --- |
| Mở coordinator | `codex` | `scripts/opencode.sh` (model `openai/gpt-6.1-sol`, `opencode.json`) |
| Chạy worker | `scripts/run-worker.sh <wave> <branch> <brief>` | `scripts/run-worker-opencode.sh <wave> <branch> <brief>` |
| Model worker | `gpt-reserve`, effort low (`~/.codex/worker.config.toml`) | `openai/gpt-6-luna`, variant low (`.opencode/agents/worker.md`) |
| Usage trong `.agent-runs/<wave>.jsonl` | `turn.completed.usage` | `step_finish.part.tokens` |

- Worker chạy liên tục tới khi mở PR, không dừng hỏi (giữ cache ấm). Script in tổng token khi xong; chép vào status board.
- OpenCode không có sandbox: agent `worker` được chạy mọi lệnh bash trừ danh sách deny (sudo, `rm -rf`,
  force-push, `gh pr merge`), chỉ trong `~/agent-ops` và `~/agent-ops-wt`.

## Quyền đã có

- Coordinator: tạo issue/label, chạy worker, comment, **merge** sau khi tự kiểm tra (ROADMAP, 24/10/2026): gate xanh
  trên đúng head (CI xanh khi đã có), `Closes #n`, không file ngoài phạm vi; `gh pr merge <n> --squash --match-head-commit <sha> --delete-branch`.
- Worker: push nhánh của mình, mở PR, comment `Status:`. Không merge.
- Chạy hết đợt không hỏi giữa chừng. Chỉ hỏi owner: quyết định sản phẩm, thao tác khó đảo ngược / rời máy,
  credentials, secret.

## Autopilot (default behaviour of the coordinator)

The loop is driven by **`scripts/autopilot.sh`** (the owner starts it once). Each call is a fresh, short
session that does **one pass** and exits; the shell sleeps between passes (no tokens spent waiting).
The **status board issue**, the issues and the PRs are the memory between passes: write there, not in chat.
Run in a terminal: `scripts/autopilot.sh` (every 10 min) · stop: `touch .agent-runs/STOP`.

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
   - **Branch sweep, every pass (both repos):** delete every remote branch except `main` whose PR is merged or
     closed and that has no open PR: `gh api -X DELETE repos/<owner>/<repo>/git/refs/heads/<branch>`. Do not rely on
     `--delete-branch` alone (it left #236/#238 behind); a branch with an open PR or a running worker is kept.
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
   **Brief = deliverable first.** Each brief states the feature to ship and its "Done when" command, and asks for
   evidence in the PR body in one fixed shape: gate output, one smoke or test result, one failing control. Prefer
   waves that ship features (`feat`) over waves that only harden smoke/CI; never open a wave whose only goal is more
   acceptance evidence for an already merged PR.
   **Run budget (owner 29/10: 65% of worker tokens went into 2nd+ runs of the same wave):** a wave gets at most
   **2 worker runs** (first + one RESUME). After that: CI green and the feature works -> merge; otherwise mark the
   issue `status:blocked`, comment why, and move to the next wave. Never start a 3rd run on the same wave.
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
   **Restart brief = old brief + a RESUME block** built cheaply from `git log origin/main..<branch>`,
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

## Language

**Everything written to the repo or GitHub is in English**: issues (title and body), PRs, commits, briefs,
comments, the status board, code comments. Vietnamese costs more tokens and the output is the expensive part.
Only the final report to the owner in chat may be Vietnamese, and short. The Vietnamese docs in this repo
are context, not a template for output language.

## Báo cáo

Ngắn. Ghi rõ đang đội mũ nào. Câu hỏi chưa giải quyết để cuối.
