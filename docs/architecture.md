# Kiến trúc

```
GitHub / GitLab ──webhook──▶ api: modules/webhook ──▶ scm provider (verify, parse) ──▶ store (Postgres)
        ▲                              │                                                 │
        └──── cmd/sync (backfill) ─────┘                                                 ▼
run-worker.sh ──POST /agent-runs──▶ api: modules/agentrun ──────────────────────▶ agent_runs
                                                                                         │
web (React) ◀── REST /api/v1/* + websocket ◀── api: modules/board, modules/runs ◀────────┘
```

## Provider pattern (theo controlplane `pkg/handlers`)

`api/pkg/handlers/scm/`:
- `provider.go`: interface `SCMProvider` + kiểu chung (`Issue`, `PullRequest`, `PipelineRun`, `Event`).
- `builder/`: registry, provider tự đăng ký trong `init()` (`builder.Register(New())`).
- `github/`, `gitlab/`: mỗi vendor một thư mục, file `github_client.go`, `github_webhook.go`, `github_mapper.go`.

Module nghiệp vụ không biết vendor: lấy provider theo `repositories.provider` rồi gọi interface. Thêm vendor
= thêm thư mục, không sửa module.

Trạng thái issue suy từ label: GitHub `status:*`, GitLab `Status::*` → enum chung
`todo | in_progress | review | blocked | done` (issue đóng = done). Mapping nằm trong mapper của provider.

## Dữ liệu (dự kiến, chốt ở W1b/W3)

| Bảng | Cột chính |
| --- | --- |
| `repositories` | id uuid, provider, full_name, webhook_secret (mã hoá hoặc env), created_at |
| `issues` | id, repository_id, number, title, state, status, labels jsonb, assignee, updated_at |
| `pull_requests` | id, repository_id, number, title, state, head_sha, branch, closes jsonb (issue numbers), merged_at |
| `pipeline_runs` | id, repository_id, pr_number nullable, sha, status, conclusion, started_at, finished_at |
| `webhook_events` | id, repository_id, provider, event_type, delivery_id unique, payload jsonb, received_at |
| `agent_runs` | id, repository_id, wave, worker, model, issue_numbers jsonb, pr_number, input/cached_input/cache_write/output/reasoning tokens, cost_usd, started_at, ended_at |

UUID khoá chính, có `created_at`/`updated_at`, index cột lọc nhiều. Webhook idempotent theo `delivery_id`.

## API

Tiền tố `/api/v1`. `/healthz`, `/readyz` ở root. Lỗi: `{"status": <code>, "message": "..."}`.
List có `?page=&per_page=`, trả `{"data": [...], "meta": {"page","per_page","total"}}`.
