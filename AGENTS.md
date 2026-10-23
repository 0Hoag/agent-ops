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
3. `docs/architecture.md`, `docs/be-standards.md`, `docs/fe-standards.md`: worker code theo đúng các file này.
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
# web/
cd web && pnpm install --frozen-lockfile && pnpm typecheck && pnpm lint && pnpm test && pnpm build
```
Chưa có thư mục nào thì gate của nó chưa áp dụng (đợt W0 tạo ra chúng). Không skip test, không tắt lint để cho xanh.

## Harness, model và chi phí

Hai harness, cùng docs và cùng skill. Chọn một cho mỗi đợt, ghi vào cột Harness của `STATUS.md`.

| | Codex CLI | OpenCode |
| --- | --- | --- |
| Mở coordinator | `codex` | `scripts/opencode.sh` (model `openai/gpt-6.1-sol`, `opencode.json`) |
| Chạy worker | `scripts/run-worker.sh <wave> <branch> <brief>` | `scripts/run-worker-opencode.sh <wave> <branch> <brief>` |
| Model worker | `gpt-reserve` (`~/.codex/worker.config.toml`) | `openai/gpt-6-luna` (`.opencode/agents/worker.md`) |
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

When started with no specific instruction (or "run", "continue", `/autopilot`), run this loop until
`ROADMAP.md` is done. The owner should never have to spell these steps out.

1. **Sync.** Read new worker output in `.agent-runs/`, `git pull`, `gh pr list`, `gh issue list --state all`.
   Commit any pending `STATUS.md` change.
2. **Plan: many small issues, all up front.** Break the whole ROADMAP into fine-grained issues before
   dispatching: one issue per concrete deliverable (one endpoint, one table + repository, one provider
   event, one page, one component, one script), each with "Done when" as a command or number. Expect
   dozens, not a handful. Labels `status:*`, `priority:*`, `type:*`, `wave:*`, `area:api|web|ci|docs`.
   A wave = several issues in the same area, given to one worker; its PR closes all of them.
   Mark dependencies (`status:blocked` + `Status:` comment).
   **Work discovered while running becomes a new issue**, never an inline fix: follow-ups, bugs, missing
   tests, a worker's "Not covered", a review finding. Label it `found-by:agent`, put it in a later wave,
   keep going.
3. **Finish open PRs.** For each worker PR: verify (gates on the PR head, CI green once it exists,
   `Closes #n`, no out-of-scope files), then merge (squash, pinned SHA), confirm the content is on `main`,
   remove its worktree. Send back with a comment instead of merging when a check fails.
4. **Dispatch.** Pick the next unblocked sub-waves. Run in parallel the ones that touch different
   directories. Write an English brief (`briefs/<wave>.md`), start the worker in the background
   (`nohup scripts/run-worker*.sh <wave> <branch> briefs/<wave>.md > .agent-runs/<wave>.log 2>&1 &`).
   Harness: OpenCode for small or UI-only waves, Codex for larger backend waves; record it.
   **At most 2 workers running at once** (ChatGPT Plus 5-hour limit). Queue the rest.
   Keep briefs precise (exact paths, line numbers, commands) so workers do not explore the repo.
5. **Wait without ending the turn.** Loop `sleep 90` + check `.agent-runs/` and `gh pr list`.
   A worker silent for 20 minutes: read its log, fix the brief, restart it.
   **Usage limit hit** (error mentions usage/rate limit, quota, 429, "try again at"): this is not an
   owner question and not a failed wave. Note the reset time from the error (or check every 15 min),
   `sleep` until then, restart the affected worker on the same branch (its pushed commits are kept),
   and continue the loop. If the coordinator itself hits the limit, the same applies on the next turn.
6. **Record.** Per wave in `STATUS.md`: harness, model, token totals printed by the script, PR, send-backs.
   New agent mistakes go to `FIVE-MINUTES.md`.
7. Back to 1. **Stop only** when the roadmap is done, or on an item from ROADMAP "Câu hỏi mở" or the
   "ask the owner" list above. Then report once: what merged, cost per wave, what is waiting on the owner.

## Language

**Everything written to the repo or GitHub is in English**: issues (title and body), PRs, commits, briefs,
comments, `STATUS.md`, code comments. Vietnamese costs more tokens and the output is the expensive part.
Only the final report to the owner in chat may be Vietnamese, and short. The Vietnamese docs in this repo
are context, not a template for output language.

## Báo cáo

Ngắn. Ghi rõ đang đội mũ nào. Câu hỏi chưa giải quyết để cuối.
