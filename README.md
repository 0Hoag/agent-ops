# agent-ops

Dashboard theo dõi quy trình **orchestrator-workers**: một agent coordinator giao issue cho các worker agent,
worker mở PR/MR, coordinator kiểm tra và merge. Agent Ops cho biết:

- board: issue nào đang ở đâu, worker nào giữ, PR/MR nào chờ CI;
- chi phí: input / output / cache read / cache write, cache hit %, $ theo worker, theo đợt, theo issue;
- chất lượng: tỉ lệ PR bị gửi lại, CI xanh lần đầu, thời gian mỗi issue.

Hỗ trợ **GitHub và GitLab** (provider pattern), để dùng được cho repo cá nhân lẫn repo công ty.

Project này cũng được xây **bằng chính quy trình orchestrator-workers** (Codex coordinator + worker).
Số liệu quá trình xây nằm trong `STATUS.md`.

## Stack

| | |
| --- | --- |
| API (`api/`) | Go, Gin, GORM + Postgres, migration SQL đánh số, Swagger (swaggo), testify, Air, golangci-lint |
| Web (`web/`) | React 18 + TypeScript, Rspack, pnpm, TanStack Query/Table, axios, react-router 6, react-hook-form, Tailwind, Vitest |

## Đọc gì trước

- Agent: `AGENTS.md` (bắt buộc), rồi `ROLES.md`, `FIVE-MINUTES.md`.
- Kiến trúc: `docs/architecture.md`. Chuẩn code: `docs/be-standards.md`, `docs/fe-standards.md`.
- Kế hoạch và trạng thái: `ROADMAP.md`, `STATUS.md`.
