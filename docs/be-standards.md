# API code standards (Go)

**Source of truth: `~/Desktop/controlplane`** (company Go service). Copy its files and conventions; adapt names
only. When this page and controlplane disagree, controlplane wins. Read these there before changing the API:

| Topic | controlplane file to follow |
| --- | --- |
| Conventions (naming, errors, file size, tests) | `docs/code-standards.md` |
| Lint config (golangci v2) | `.golangci.yml` (drop the govmomi errcheck exclusion) |
| Make targets, LDFLAGS version/commit | `Makefile` (`build run dev install fmt vet lint test test-coverage ci swagger docker-build`) |
| Hot reload | `.air.toml` |
| Wiring modules in `main.go` | `main.go` (NewRepository → NewController → NewRoutes(...).Register(group)) |
| Module layout | `modules/team/` (models, types, repository*, controller*, routes, README) |
| Response and error JSON | `pkg/response/response.go`, `pkg/response/error.go` |
| Pagination | `pkg/pagination/pagination.go` |
| Middleware | `pkg/middleware/requestid.go`, `pkg/middleware/security.go` |
| Config / logger | `pkg/config/config.go`, `pkg/logger/logger.go` |
| Migrations | `migrations/NNN_create_<thing>_table.sql` (numbered, never edit a merged one) |
| Provider pattern | `pkg/handlers/` (interface + `builder` registry + one dir per vendor, `init()` registration) |
| Swagger | swaggo annotations on handlers, served at `/swagger/*` |

## Layout in this repo

```
api/
├── main.go
├── cmd/<tool>/main.go          # agentops CLI, sync, ...
├── modules/<name>/             # models.go types.go repository*.go controller*.go routes.go README.md (+ service.go only when needed)
├── pkg/{config,database,logger,middleware,pagination,response}
├── pkg/handlers/scm/           # SCMProvider + builder + github/ gitlab/
├── migrations/NNN_<verb>_<thing>_table.sql
└── Makefile .air.toml .golangci.yml Dockerfile
```

## Rules that matter most (from controlplane `docs/code-standards.md`)

- `gofmt` + `goimports`; `golangci-lint` with the copied config is a gate. Files `snake_case.go`, packages lowercase.
- **≤200 lines per file**; split by entity/feature (`controller_<feature>.go`, `repository_<entity>.go`).
- Constructor injection, `context.Context` passed down from `gctx.Request.Context()`.
- Repository verbs: `Get`, `List`, `Create`, `Update`, `Delete`, `Exists`, `Count`. Booleans `IsX`/`HasX`.
- Errors wrapped with `fmt.Errorf("...: %w", err)`; handlers answer through `pkg/response`, never leak DB errors.
- Table-driven tests with testify; provider payloads in `testdata/`; a test must be shown to fail once.
- Secrets only from env; never log tokens or full payloads.
- It must run: `make ci` and `scripts/smoke.sh` stay green (ROADMAP, 24/10/2026).
