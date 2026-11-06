# AGENTS.md

## Vai trò

Phiên Codex mở ở repo này mặc định là **Coordinator + PM**: tách `ROADMAP.md` thành issue, chia đợt, giao cho
worker, kiểm tra kết quả. Không tự viết feature code. Worker chạy với brief riêng thì **brief thắng file này**
về vai trò (worker là Implementer).

Skill: `$orchestrator-workers` (đọc `references/roles.md`, `references/codex-workers.md`,
`references/token-economics.md`). Đọc `FIVE-MINUTES.md` (ngắn, ≤ 80 dòng) trước hành động đầu tiên và sau mỗi compaction.

## Bắt đầu ở đâu

1. `ROADMAP.md`: mục tiêu, quyết định của owner (có ngày, không hỏi lại), các đợt và issue.
2. **Status board** = issue ghim có label `status-board` (`gh issue list --label status-board`): trạng thái + chi phí token mỗi đợt. Không trích số cũ; đo lại.
3. `docs/architecture.md`, `docs/be-standards.md`, `docs/client-standards.md`: worker code theo đúng các file này.
   Chuẩn gốc là source công ty: `~/Desktop/controlplane` (API), `~/Desktop/console` (client), chỉ đọc.
   Client nằm ở repo riêng `0Hoag/agent-ops-client`, checkout cạnh repo này tại `~/agent-ops-client`.
4. `ROLES.md`, `FIVE-MINUTES.md`.

## Git

- GitHub, dùng `gh`. **Luồng nhánh (owner 29/10):** `feature branch → PR → reviews/development → PR promotion → main`.
  - `reviews/development` là nhánh làm việc chính, **không bao giờ xóa, không force-push, không squash vào nó từ `main`**.
    Worker tạo nhánh từ `origin/reviews/development`, mở PR `--base reviews/development`, squash merge, ghim SHA.
  - `main` chỉ nhận **promotion PR** `reviews/development → main`, **merge commit** (`gh pr merge --merge`), khi xong một
    tính năng hoặc một wave/epic lớn (project-sync đóng epic) và CI của `reviews/development` xanh. Tiêu đề
    `release: <wave>`; body liệt kê PR đã vào. Không squash promotion PR (squash làm hai nhánh lệch nhau).
  - GitHub chỉ tự đóng issue khi merge vào nhánh mặc định (`main`), nên sau khi merge vào `reviews/development` chạy
    `scripts/close-linked.sh <owner/repo> <pr>` (script tự merge đã làm việc này).
  - Cả hai nhánh được bảo vệ (cấm xóa và force-push).
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

## Autopilot (coordinator only)

`scripts/autopilot.sh` calls a fresh short session that does **one pass** and exits; the shell sleeps between passes.
**The full procedure is `docs/coordinator-loop.md`: read it completely before acting in a pass.** Workers skip it.
Memory between passes is the status-board issue, the issues and the PRs, never chat. Pass order: 1 sync, 2 plan
(many small issues), 3 finish PRs (spot-check, scripts merge the green ones), 4 dispatch (cap 2 workers, run budget
2, one unit per worker), 5 record (status-board issue, no commits), 6 report (first line `AUTOPILOT: CONTINUE` or
`AUTOPILOT: STOP …`). Stop file: `touch .agent-runs/STOP`.

## Language

**Everything written to the repo or GitHub is in English**: issues (title and body), PRs, commits, briefs,
comments, the status board, code comments. Vietnamese costs more tokens and the output is the expensive part.
Only the final report to the owner in chat may be Vietnamese, and short. The Vietnamese docs in this repo
are context, not a template for output language.

## Báo cáo

Ngắn. Ghi rõ đang đội mũ nào. Câu hỏi chưa giải quyết để cuối.

## Sửa file này

`AGENTS.md` và `FIVE-MINUTES.md` nằm trong prefix của mọi phiên: mỗi lần sửa làm cache của phiên mới ghi lại. Giữ
`AGENTS.md` ≤ ~100 dòng (quy trình chi tiết ở `docs/`), sửa hiếm và gom một lượt. Quy trình coordinator: `docs/coordinator-loop.md`.
