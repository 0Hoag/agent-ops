# Chuẩn code API (Go)

Rút gọn từ `controlplane/docs/code-standards.md` của công ty. Không rõ thì làm theo cách gần nhất đã có trong `api/`.

## Bố cục

```
api/
├── main.go                    # nối module: NewRepository(db) → NewController(repo) → NewRoutes(ctrl).Register(v1)
├── cmd/<tool>/main.go         # công cụ dòng lệnh (sync, ...)
├── modules/<tên>/             # mỗi feature một module
│   ├── models.go              # GORM model
│   ├── types.go               # DTO request/response
│   ├── repository.go          # chỉ truy vấn DB; tách repository_<entity>.go khi lớn
│   ├── controller.go          # handler Gin; tách controller_<feature>.go khi lớn
│   ├── service.go             # logic nghiệp vụ (chỉ khi có, không tạo trước)
│   ├── routes.go              # type Routes; func (r *Routes) Register(g *gin.RouterGroup)
│   └── README.md              # vài dòng: module làm gì, endpoint
├── pkg/{config,database,response,pagination,logger,middleware}
├── pkg/handlers/scm/          # provider pattern, xem docs/architecture.md
└── migrations/NNN_<mô_tả>.sql # đánh số tăng dần, không sửa file đã merge
```

## Luật

- `gofmt`; `golangci-lint` là gate. Package chữ thường không gạch dưới; file `snake_case.go`.
- **File ≤200 dòng.** Lớn hơn thì tách theo entity/feature. Module > ~6 file thì tách sub-package.
- Dependency truyền qua constructor (`NewController(repo *Repository)`); không biến toàn cục (trừ registry provider).
- Luôn truyền `context.Context` (`gctx.Request.Context()`) xuống repository/provider.
- Động từ repository: `Get` (theo id), `List` (phân trang), `Create`, `Update`, `Delete`, `Exists`, `Count`.
- Boolean: `IsX`, `HasX`. Tên biến đầy đủ (`issue`, không `i`), trừ `ctx`, `err`, `id`.
- Lỗi: `fmt.Errorf("get issue %d: %w", n, err)`; controller trả `response.Error(gctx, status, msg)`, không lộ lỗi DB.
- Response list: `response.List(gctx, data, meta)`. Không trả struct GORM trực tiếp nếu có trường nội bộ.
- Swagger annotation trên mọi handler public.
- Secret chỉ từ env; không log secret, token, payload đầy đủ.

## Test

- `testify` + test dạng bảng (table-driven). File `<tên>_test.go` cạnh code.
- Provider: payload mẫu trong `testdata/`, có ca chữ ký sai.
- Repository: `go-sqlmock` hoặc Postgres docker (đánh dấu integration bằng build tag `integration`).
- Test phải fail được: đổi một dòng logic, thấy đỏ, hoàn nguyên (trong worktree, không `git checkout --`).
