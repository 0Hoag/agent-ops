# agent-ops

Dashboard theo dõi quy trình **orchestrator-workers**: một agent coordinator giao issue cho các worker agent,
worker mở PR/MR, coordinator kiểm tra và merge. Agent Ops cho biết:

- board: issue nào đang ở đâu, worker nào giữ, PR/MR nào chờ CI;
- chi phí: input / output / cache read / cache write, cache hit %, $ theo worker, theo đợt, theo issue;
- chất lượng: tỉ lệ PR bị gửi lại, CI xanh lần đầu, thời gian mỗi issue.

Hỗ trợ **GitHub và GitLab** (provider pattern), để dùng được cho repo cá nhân lẫn repo công ty.

Project này cũng được xây **bằng chính quy trình orchestrator-workers** (Codex coordinator + worker).
Số liệu quá trình xây nằm trong issue ghim **Status board** (label `status-board`).

## Stack

| | |
| --- | --- |
| API (`api/`) | Go, Gin, GORM + Postgres, migration SQL đánh số, Swagger (swaggo), testify, Air, golangci-lint |
| Client (`0Hoag/agent-ops-client`, sibling checkout `../agent-ops-client`) | React 18 + TypeScript, Rspack, pnpm, TanStack Query/Table, axios, react-router 6, react-hook-form, Tailwind, Vitest |

## Đọc gì trước

- Agent: `AGENTS.md` (bắt buộc), rồi `ROLES.md`, `FIVE-MINUTES.md`.
- Kiến trúc: `docs/architecture.md`. Chuẩn code: `docs/be-standards.md`, `docs/client-standards.md`.
- Kế hoạch: `ROADMAP.md`; trạng thái hiện tại: issue ghim Status board.

## Local API development

Prerequisites: Docker with Compose and Go. The default host API connection is
`postgres://agent_ops:agent_ops_dev@localhost:5432/agent_ops?sslmode=disable`.
Start the development database with `docker compose up -d postgres`. Set the
variables from `api/.env.example` in your shell, then run the host API from
`api/` with `go run .`. The database is stored in the persistent
`agent_ops_postgres` volume. To avoid a host port collision, choose an unused
port with `POSTGRES_HOST_PORT=15432 docker compose up -d postgres` and set
`DATABASE_URL=postgres://agent_ops:agent_ops_dev@localhost:15432/agent_ops?sslmode=disable`
for the host API. Do not use the Compose-internal hostname `postgres` from a
host process.

## Run everything locally

Prerequisites: Docker with Compose, Node.js 20.19+ and pnpm 11.21+, and Chromium available to Playwright (`pnpm exec playwright install chromium` once in the client checkout if needed). The smoke uses `~/agent-ops-client` by default, cloning it if absent and only fast-forwarding a clean checkout. To avoid modifying a canonical checkout with user changes, use a clean disposable client clone and set `CLIENT_DIR` as shown below. No external credentials or shared database are used; the smoke creates an isolated Postgres/API stack and disposable repository, issue, pull request, and CI fixtures.

From a fresh `agent-ops` worktree, run these commands (the smoke clones the client automatically when it does not exist):

```sh
make smoke
```

For a strictly disposable client checkout, run these commands from this repository (the clone location must not already exist):

```sh
git clone https://github.com/0Hoag/agent-ops-client.git ../agent-ops-client-smoke
CLIENT_DIR="$(pwd)/../agent-ops-client-smoke" make smoke
```

The command builds and previews the client against the API, launches Chromium to assert the visible seeded issue and linked PR/CI plus successful API network responses, and saves logs/screenshot under `.agent-runs/smoke/`. It always stops its preview process and removes its uniquely named containers and volume; persistent development Compose data is not touched. If the sibling checkout already exists, it is fetched/fast-forwarded only when clean; user changes are preserved by refusing to update. API and client ports can be overridden with `SMOKE_API_PORT` and `SMOKE_CLIENT_PORT`.
