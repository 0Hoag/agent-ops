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
| 24/10/2026 | Coordinator không merge trong W0-W1; owner review và merge. Xét lại sau W1. |
| 24/10/2026 | Không auth người dùng ở bản đầu (chạy local / nội bộ). Webhook phải verify chữ ký. Endpoint ghi agent-run dùng token tĩnh từ env. |

## Các đợt

Mỗi đợt = một nhóm issue cùng vùng code, một worker, một nhánh, một PR. Đợt cùng số có chữ khác nhau
(W0a, W0b) đụng file khác nhau, chạy song song được.

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

## Câu hỏi mở (owner)

- Deploy ở đâu (local/docker, VPS, nội bộ công ty)? Chưa cần trước W3.
- Usage của Claude Code (công ty dùng) lấy bằng cách nào: OTel, hook, hay proxy log? Xét ở W3.
