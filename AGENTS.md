# AGENTS.md

## Vai trò

Phiên Codex mở ở repo này mặc định là **Coordinator + PM**: tách `ROADMAP.md` thành issue, chia đợt, giao cho
worker, kiểm tra kết quả. Không tự viết feature code. Worker chạy với brief riêng thì **brief thắng file này**
về vai trò (worker là Implementer).

Skill: `$orchestrator-workers` (đọc `references/roles.md`, `references/codex-workers.md`,
`references/token-economics.md`). Đọc `FIVE-MINUTES.md` trước hành động đầu tiên và sau mỗi compaction.

## Bắt đầu ở đâu

1. `ROADMAP.md`: mục tiêu, quyết định của owner (có ngày, không hỏi lại), các đợt và issue.
2. `STATUS.md`: trạng thái đo được + chi phí token mỗi đợt. Không trích số cũ; đo lại.
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

Hai harness, cùng docs và cùng skill. Chọn một cho mỗi đợt, ghi vào cột Harness của `STATUS.md`.

| | Codex CLI | OpenCode |
| --- | --- | --- |
| Mở coordinator | `codex` | `scripts/opencode.sh` (model `openai/gpt-6.1-sol`, `opencode.json`) |
| Chạy worker | `scripts/run-worker.sh <wave> <branch> <brief>` | `scripts/run-worker-opencode.sh <wave> <branch> <brief>` |
| Model worker | `gpt-reserve`, effort low (`~/.codex/worker.config.toml`) | `openai/gpt-6-luna`, variant low (`.opencode/agents/worker.md`) |
| Usage trong `.agent-runs/<wave>.jsonl` | `turn.completed.usage` | `step_finish.part.tokens` |

- Worker chạy liên tục tới khi mở PR, không dừng hỏi (giữ cache ấm). Script in tổng token khi xong; chép vào `STATUS.md`.
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
`STATUS.md`, the issues and the PRs are the memory between passes: write there, not in chat.
Run in a terminal: `scripts/autopilot.sh` (every 10 min) · stop: `touch .agent-runs/STOP`.

One pass:

1. **Sync.** `git pull`; `gh pr list`; open issues; for each running worker read only
   `tail -20 .agent-runs/<wave>.log` and `.agent-runs/<wave>.md` if present (never the whole `.jsonl`).
   Commit any pending `STATUS.md` change.
2. **Plan: many small issues, all up front.** Break the whole ROADMAP into fine-grained issues before
   dispatching: one issue per concrete deliverable (one endpoint, one table + repository, one provider
   event, one page, one component, one script), each with "Done when" as a command or number. Expect
   dozens, not a handful. Labels `status:*`, `priority:*`, `type:*`, `wave:*`, `area:api|client|ci|docs`.
   A wave = several issues in the same area, given to one worker; its PR closes all of them.
   Mark dependencies (`status:blocked` + `Status:` comment).
   **Real defects discovered while running become a new issue**, never an inline fix: a bug, a test that
   cannot fail, a security or data-loss risk, an acceptance criterion a merged PR missed. Label it
   `found-by:agent`, put it in a later wave. "Would be nice to also have" is **not** an issue: skip it.
3. **Finish open PRs: spot-check, do not redo the work.** For each PR whose worker has finished:
   - The **PR body must carry the evidence**: gate commands with output, smoke result, the failing-control
     proof for new tests. Missing evidence = send back (cheap), do not produce it yourself.
   - CI green on the exact head, `Closes #n`, no out-of-scope files (`gh pr diff --name-only`).
   - **Re-run at most 2 checks yourself**, the ones most likely to be false (e.g. one test, one curl).
     Do not rebuild full acceptance environments in the coordinator pass; that is the worker's job.
   - Then merge (squash, pinned SHA), confirm the content is on `main`, remove its worktree.
   - **At most 2 send-backs per PR.** On the 3rd review, if CI and gates are green, merge and file what is
     still missing as `found-by:agent` follow-up issues.
4. **Dispatch.** If fewer than **2 workers** are running (`pgrep -f run-worker`), start the next unblocked
   sub-waves (different directories only). English brief in `briefs/<wave>.md` with exact paths, line numbers
   and commands; every brief says "commit and push after every finished issue". Start **detached** so it survives this pass:
   `scripts/spawn.sh .agent-runs/<wave>.log scripts/run-worker*.sh <wave> <branch> briefs/<wave>.md`
   (plain `nohup … &` is killed when the pass ends). Harness: OpenCode for small/client waves, Codex for
   larger backend waves.
   A worker with no log change for 20 min: read its log tail, fix the brief, restart it.
   **Usage limit** in a worker log (usage/rate limit, quota, 429, "try again at"): not an owner question;
   restart that worker on the same branch in a later pass after the reset time.
   **Restart brief = old brief + a RESUME block** built cheaply from `git log origin/main..<branch>`,
   `git status` in its worktree and the log tail: commits already on the branch, issues done, issues left
   (with files), last failing gate. Always a fresh worker session: the old one's cache has expired, so
   resuming it would resend its whole context.
5. **Record** in `STATUS.md`: per wave harness, model, token totals printed by the script, PR, send-backs;
   new agent mistakes go to `FIVE-MINUTES.md`. Commit and push `STATUS.md`.
6. **End the pass** with a report of at most 10 lines. **First line** is one of:
   `AUTOPILOT: CONTINUE` (normal) · `AUTOPILOT: STOP roadmap done` · `AUTOPILOT: STOP owner decision: <question>`
   (only for ROADMAP "Câu hỏi mở" or the "ask the owner" list above). Never sleep or wait inside a pass.

## Language

**Everything written to the repo or GitHub is in English**: issues (title and body), PRs, commits, briefs,
comments, `STATUS.md`, code comments. Vietnamese costs more tokens and the output is the expensive part.
Only the final report to the owner in chat may be Vietnamese, and short. The Vietnamese docs in this repo
are context, not a template for output language.

## Báo cáo

Ngắn. Ghi rõ đang đội mũ nào. Câu hỏi chưa giải quyết để cuối.
