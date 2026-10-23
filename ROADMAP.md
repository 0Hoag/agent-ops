# Roadmap

## Mục tiêu

1. Một dashboard dùng thật cho quy trình orchestrator-workers: board, chi phí token, chất lượng.
2. Dùng được cho **repo công ty (GitLab)**, không chỉ GitHub cá nhân.
3. Chính quá trình xây là thí nghiệm: đo chi phí và chất lượng của orchestrator-workers trên Codex
   (ghi trong `STATUS.md`), để quyết định có mang về công ty không.

## Quyết định của owner (có ngày, không hỏi lại)

| Ngày | Quyết định |
| --- | --- |
| 24/10/2026 | Stack: API Go theo flow của controlplane (Gin, GORM, module controller/repository/routes); web React theo console (Rspack, pnpm, TanStack Query). |
| 24/10/2026 | GitHub làm trước; **GitLab provider là bắt buộc** (để dùng ở công ty). Thiết kế provider interface ngay từ W1. |
| 24/10/2026 | ~~Coordinator không merge trong W0-W1~~ (thay bằng dòng dưới). |
| 24/10/2026 | **Coordinator tự merge** sau khi tự kiểm tra (gate xanh trên đúng head, CI xanh khi đã có W0c, `Closes #n`, không file ngoài phạm vi), squash ghim SHA. Chạy liên tục hết ROADMAP, không chờ owner. Worker vẫn không merge. |
| 24/10/2026 | PM tách **toàn bộ ROADMAP thành nhiều issue nhỏ** ngay (mỗi issue một deliverable, xem AGENTS.md → Autopilot bước 2); việc phát sinh thành issue mới `found-by:agent`. |
| 24/10/2026 | Deploy: **local, docker compose** (API + web + Postgres). VPS / hạ tầng công ty quyết sau demo, không làm trong W0-W5. |
| 24/10/2026 | Usage Claude Code (W3): **đọc transcript local `~/.claude/projects/**/*.jsonl`** (usage mỗi message), cùng cách với Codex / OpenCode (đọc JSON). OTel và log proxy công ty để đợt sau, tạo issue `found-by:agent` nếu cần. |
| 24/10/2026 | Không auth người dùng ở bản đầu (chạy local / nội bộ). Webhook phải verify chữ ký. Endpoint ghi agent-run dùng token tĩnh từ env. |
| 24/10/2026 | **The frontend is called `client` (not `web`) and moves to its own repository `0Hoag/agent-ops-client` (public).** `agent-ops` keeps the Go API, `agentops` CLI, compose, docs. Done in wave WS, right after the in-flight W2b PR. |
| 24/10/2026 | **Code format follows the company sources literally**: API = `~/Desktop/controlplane` (config files, Makefile targets, package layout, response/error shape), client = `~/Desktop/console` (package scripts, tsconfig, rspack config, module layout, code style). Copy their config files and adapt; do not invent a parallel style. Details: `docs/be-standards.md`, `docs/client-standards.md`. |
| 24/10/2026 | **It must run.** Every wave that touches API or web keeps `scripts/smoke.sh` green: compose up → API healthy → web built and served → the page loads real data from the API. A PR that breaks the smoke is sent back. |

## Các đợt

Mỗi đợt = một nhóm issue cùng vùng code, một worker, một nhánh, một PR. Đợt cùng số có chữ khác nhau
(W0a, W0b) đụng file khác nhau, chạy song song được.

### WS: split web repo, conform to company sources, run end to end (priority over W2+)

Run after the in-flight W2b PR merges; pause other web waves until WS-a is done.

| Sub-wave | Issues (one deliverable each) | Done when |
| --- | --- | --- |
| WS-a `split` | rename everywhere `web` → `client`: label `area:web` → `area:client` (`gh label edit`), issue titles/bodies mentioning web, wave names, docs, CI job name, compose service, `run-worker*.sh` (`web/node_modules` → client checkout), smoke · create `0Hoag/agent-ops-client` (public) · move `web/` **with history** (`git subtree split --prefix=web`) · copy agent docs into it (AGENTS.md adapted to client-only gates, ROLES.md, FIVE-MINUTES.md, docs/client-standards.md, opencode.json, .opencode/, scripts/) · its own CI (typecheck, lint, test, build) · `gh issue transfer` every open `area:client` issue to agent-ops-client · remove `web/` and the `web` CI job from `agent-ops` · update README/architecture of both | both repos CI green on `main`; `git log` in agent-ops-client shows the old web commits; no `area:client` issue left open in agent-ops |
| WS-b `api-conform` | copy controlplane `.golangci.yml` (v2 config: govet, errcheck, ineffassign, staticcheck SA*, unused; gofmt+goimports) · Makefile with controlplane targets (`build run dev install fmt vet lint test test-coverage ci swagger docker-build`, LDFLAGS version/commit) · `.air.toml` like controlplane · `api/migrations/NNN_<verb>_<thing>_table.sql` · `pkg/response/{response,error}.go`, `pkg/pagination`, `pkg/middleware/{requestid,security}.go` · CORS via gin-contrib/cors · Swagger at `/swagger/*` · every module = models/types/repository*/controller*/routes/README | `make ci` green; `golangci-lint run` with the copied config = 0 issues; `/swagger/index.html` 200 |
| WS-c `client-conform` (in agent-ops-client) | console scripts (`dev: RSPACK_MODE=local rspack serve`, `build: tsc --noEmit && … rspack build`, `typecheck`, `test: vitest run`, `preview`) · `packageManager` pnpm pinned, `.nvmrc` 22, engines node >=20.19 · tsconfig compiler options as console + `@/*` alias · rspack.config.ts loading `.env`, `.env.<mode>`, `.env.local` (dotenv) with `APP_API_URL` · module layout `src/app`, `src/modules/<m>/{module,route,paths}.ts`, `services/client.ts` as the only HTTP seam · console code style (single quotes, semicolons, 2 spaces, doc comment on exported functions) | all scripts green; `.env.example` documents every variable |
| WS-d `run-e2e` | `scripts/smoke.sh` in agent-ops: compose up Postgres + API, wait healthy, clone/pull agent-ops-client next to it (`~/agent-ops-client`), `pnpm build && pnpm preview`, Playwright (or curl + HTML check) that the board page renders data from the API · `make smoke` · README "Run everything locally" (≤ 5 commands) | `make smoke` exits 0 on a clean checkout; screenshot/log attached to the PR |

### W0: nền móng (owner theo dõi sát)

| Đợt | Issue | Xong khi |
| --- | --- | --- |
| W0a `api` | Khung `api/`: Gin, `/healthz`, `/readyz` (ping DB), config từ env (`.env.example`), GORM Postgres, runner migration SQL đánh số, `pkg/{config,database,response,logger,middleware}`, Makefile (`dev test lint build ci`), `.air.toml`, `.golangci.yml`, Dockerfile; `docker-compose.yml` ở root chạy Postgres | `make ci` xanh; `docker compose up -d && make dev` rồi `curl :8080/healthz` = 200, `/readyz` = 200 |
| W0b `web` | Khung `web/`: Rspack + React 18 + TS strict, pnpm, TanStack Query, react-router 6, Tailwind, Vitest, ESLint; `src/app` (router, layout sidebar), `src/shared/http` (axios), một module mẫu `modules/health` gọi `/healthz` | `pnpm typecheck && pnpm lint && pnpm test && pnpm build` xanh; trang health hiện trạng thái API |
| W0c `ci` (sau W0a, W0b) | `.github/workflows/ci.yml`: job `api` (Go: vet, golangci-lint, test, build), job `web` (pnpm: typecheck, lint, test, build) | CI xanh trên `main`; một PR cắm lỗi type thấy đỏ (rồi đóng) |

### W1: dữ liệu vào (GitHub)

| Đợt | Issue | Xong khi |
| --- | --- | --- |
| W1a `scm` | `pkg/handlers/scm`: interface `SCMProvider` (verify webhook, parse event → kiểu chung Issue/PullRequest/PipelineRun, list cho backfill) + registry; provider `github` (HMAC `X-Hub-Signature-256`, event `issues`, `pull_request`, `workflow_run`) | test bảng với payload mẫu trong `testdata/`; chữ ký sai → lỗi |
| W1b `store` | Migration + model + repository: `repositories`, `issues` (labels, status suy từ `status:*`), `pull_requests` (closes issue ids, head sha, state), `pipeline_runs`, `webhook_events` (raw, để replay) | test repository (sqlmock hoặc Postgres docker) |
| W1c `ingest` (sau W1a, W1b) | `modules/webhook`: `POST /api/v1/webhooks/:provider` → verify → lưu raw → upsert; `cmd/sync`: backfill một repo qua API | gửi payload mẫu bằng curl → bản ghi đúng trong DB; chạy `cmd/sync` với repo `agent-ops` này |

### W2: board

`modules/board` API (issue theo cột status, PR và CI kèm theo, filter theo repo) · web `modules/board`
(cột Todo/In progress/Review/Blocked/Done, card có PR + CI) · realtime qua websocket khi có webhook.

### W3: agent run và chi phí

Bảng `agent_runs` (wave, worker, model, issue ids, PR, input/output/cache read/cache write, cost, thời gian) ·
`POST /api/v1/agent-runs` (token tĩnh) · `scripts/run-worker.sh` gửi usage từ `codex exec --json` · bảng giá
model trong config · web `modules/runs`: cache hit %, $ theo đợt/worker/issue.

### W4: GitLab provider

Provider `gitlab` (header `X-Gitlab-Token`, event Issue/Merge Request/Pipeline, label `Status::*`) · backfill
qua API GitLab · cấu hình nhiều repo, mỗi repo một provider.

### W5: insight

Tỉ lệ PR bị gửi lại, CI xanh lần đầu, thời gian mỗi issue, gợi ý mục `FIVE-MINUTES.md` từ lỗi lặp lại.

## Phase 2 (W6-W17): make it a product the company can adopt

Added 24/10/2026 by the owner: the project must be large enough to exercise the loop for days, not hours.
Same rules: split each wave into many small issues (one deliverable each, "Done when" as a command/number),
group by area, parallel where directories differ. Order below is the default; dependencies win.

### W6: API contract and generated client
OpenAPI from swaggo for every endpoint · `make swagger` in CI (fails on drift) · generate a typed TS client
into `web/src/shared/api` · replace hand-written axios calls · contract test: every route has a schema.
Done when: CI fails if a handler changes without regenerating; the client app uses only the generated API client.

### W7: auth, organizations, RBAC
Local accounts (bcrypt) + JWT sessions · organizations, members, roles `owner|admin|member|viewer` ·
every query scoped by `organization_id` from the token (tenant isolation, like controlplane) · repositories
belong to an org · invite links · web: login, org switcher, members page, role-based UI.
Done when: a test proves org A cannot read org B's issues, runs or costs through any endpoint.

### W8: multi-repo, multi-provider settings
Repository CRUD in the UI (provider, URL, webhook secret encrypted at rest with AES-GCM, token for backfill) ·
per-repo status-label mapping (GitHub `status:*`, GitLab `Status::*`, custom) · webhook URL + secret shown
once · GitLab self-hosted base URL. Done when: one org tracks this GitHub repo and a GitLab repo side by side.

### W9: agent run ingestion from every harness
`agentops` CLI (Go, `api/cmd/agentops`): `agentops push --harness codex|opencode|claude <file>` parses
Codex `turn.completed`, OpenCode `step_finish`, Claude Code transcripts · idempotent by run/session id ·
`run-worker*.sh` call it · `agentops watch ~/.claude/projects` tails new transcripts.
Done when: the same session pushed twice is stored once; fixtures for all three formats in `testdata/`.

### W10: pricing and cost engine
Model price table (input, cached input, cache write, output, reasoning) editable in UI, versioned by date ·
cost computed per run at the price valid on that day · subscription harnesses (cost 0) shown as
"equivalent API cost" · currency USD/VND. Done when: unit tests cover cache-heavy and reasoning-heavy runs.

### W11: run explorer and session timeline
Runs list (filter by wave, harness, model, issue, PR, date) · run detail: timeline of turns/steps with
tokens per step, tool calls, cache hit per step, cost curve · link run ↔ issue ↔ PR ↔ CI.
Done when: a 50-turn run renders in under 1 s with 10k runs in the DB (seed script + measurement).

### W12: analytics dashboards
Cost per day/week by harness, model, wave, area · cache-hit trend · cost per merged PR / per closed issue ·
send-back rate, first-pass CI rate, lead time issue→merge · harness comparison normalised by diff size.
Done when: every number on screen has a test that recomputes it from fixtures.

### W13: budgets and alerts
Budgets per org/repo/wave (tokens or money) · alerts: budget 80/100 %, cache hit below threshold, worker
silent > N min, CI red on main, PR open > N h · channels: Telegram bot, Slack webhook, email (SMTP) ·
alert history page. Done when: each rule has a test that fires it and one that does not.

### W14: coordinator control panel
Wave board in the UI (Todo → In progress → Review → Done per wave) · start/stop/restart a worker from the UI
(API runs `run-worker*.sh` through a job queue with logs streamed over websocket) · brief editor with
templates · audit log of every action. Done when: a wave can be dispatched and watched end-to-end from the UI.

### W15: MCP server for agents
`agentops mcp` exposes read tools: `list_issues`, `wave_status`, `my_costs`, `similar_failures` (search
`FIVE-MINUTES.md` and past send-backs) so coordinators and workers can query Agent Ops themselves.
Done when: an OpenCode session calls the tools and gets real data (recorded in the PR).

### W16: quality and operations
Playwright E2E for login, board, runs, alerts · Prometheus `/metrics` (request latency, webhook lag, queue
depth) · structured request logs with request id · rate limiting on webhooks · backup/restore script for
Postgres · load test (k6): 50 webhooks/s sustained. Done when: E2E in CI, k6 report in the PR.

### W17: release
Docker images for API and web (multi-stage, non-root) · `docker compose -f compose.prod.yml up` runs the
whole stack · semantic versioning + CHANGELOG from commits · docs: install, connect GitHub/GitLab, connect
each harness, upgrade. Done when: a clean machine goes from clone to working dashboard in under 10 minutes
following only the docs (a worker does it in a fresh worktree and reports the time).

## Câu hỏi mở (owner)

Không còn (đã trả lời 24/10/2026, xem bảng quyết định).
