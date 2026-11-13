package repositorysettings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/pkg/crypto"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/0Hoag/agent-ops/api/modules/store"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRoutesRegisterAllSettingsOperations(t *testing.T) {
	r := gin.New()
	NewRoutes(&Controller{}).Register(r.Group("/api/v1"))
	routes := r.Routes()
	got := make(map[string]bool, len(routes))
	for _, route := range routes {
		got[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/repositories",
		"POST /api/v1/repositories",
		"POST /api/v1/repositories/test",
		"GET /api/v1/repositories/:repository_id",
		"PATCH /api/v1/repositories/:repository_id",
		"DELETE /api/v1/repositories/:repository_id",
	} {
		require.True(t, got[route], "missing settings route %s", route)
	}
}

func TestViewRedactsStoredCredentialsOnSubsequentReads(t *testing.T) {
	c := &Controller{webhookBase: "https://agent.example"}
	r := &store.Repository{
		ID: "11111111-1111-1111-1111-111111111111", Provider: "github", FullName: "acme/app",
		WebhookSecretCiphertext: "cipher-secret", BackfillTokenCiphertext: "cipher-token",
		StatusMappings: store.JSONB(`{"status:review":"review"}`),
	}
	v := c.view(r)
	b, err := json.Marshal(v)
	require.NoError(t, err)
	require.NotContains(t, string(b), "cipher-secret")
	require.NotContains(t, string(b), "cipher-token")
	require.NotContains(t, string(b), "webhook_secret")
	require.NotContains(t, string(b), "backfill_token")
	if v.WebhookURL != "https://agent.example/api/v1/webhooks/github" {
		t.Fatalf("unexpected webhook URL %q", v.WebhookURL)
	}
	if v.StatusMappings["status:review"] != "review" {
		t.Fatalf("status mappings missing: %#v", v.StatusMappings)
	}
}

func TestTestRepositoryEndpointExercisesProviderAndAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		membership bool
		body       string
		status     int
		provider   int
	}{
		{name: "success", role: organization.RoleOwner, membership: true, body: `{"provider":"github","full_name":"acme/app","base_url":"PLACEHOLDER","token":"secret-token"}`, status: http.StatusOK, provider: http.StatusOK},
		{name: "invalid input", role: organization.RoleOwner, membership: true, body: `{"provider":"github","full_name":"","base_url":"http://unused","token":"secret-token"}`, status: http.StatusBadRequest},
		{name: "unauthorized", role: organization.RoleMember, membership: true, body: `{"provider":"github","full_name":"acme/app","base_url":"PLACEHOLDER","token":"secret-token"}`, status: http.StatusForbidden},
		{name: "bad token", role: organization.RoleOwner, membership: true, body: `{"provider":"github","full_name":"acme/app","base_url":"PLACEHOLDER","token":"secret-token"}`, status: http.StatusUnprocessableEntity, provider: http.StatusUnauthorized},
		{name: "missing project", role: organization.RoleOwner, membership: true, body: `{"provider":"github","full_name":"acme/app","base_url":"PLACEHOLDER","token":"secret-token"}`, status: http.StatusNotFound, provider: http.StatusNotFound},
		{name: "network failure", role: organization.RoleOwner, membership: true, body: `{"provider":"github","full_name":"acme/app","base_url":"http://127.0.0.1:1","token":"secret-token"}`, status: http.StatusBadGateway},
		{name: "missing membership", role: organization.RoleOwner, membership: false, body: `{"provider":"github","full_name":"acme/app","base_url":"http://unused","token":"secret-token"}`, status: http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var provider *httptest.Server
			if tc.provider != 0 {
				provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, "/repos/acme/app", r.RequestURI)
					require.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
					w.WriteHeader(tc.provider)
					if tc.provider == http.StatusOK {
						_, _ = w.Write([]byte(`{"full_name":"acme/app"}`))
					}
				}))
				t.Cleanup(provider.Close)
				tc.body = bytes.NewBufferString(tc.body).String()
				tc.body = string(bytes.Replace([]byte(tc.body), []byte("PLACEHOLDER"), []byte(provider.URL), 1))
			}
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
			require.NoError(t, err)
			if tc.membership {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2`)).WithArgs("org-a", "account-a", 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "role"}).AddRow("membership", "org-a", "account-a", tc.role))
			} else {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2`)).WithArgs("org-a", "account-a", 1).
					WillReturnRows(sqlmock.NewRows([]string{"id"}))
			}
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("organization_id", "org-a"); c.Set("account_id", "account-a"); c.Next() })
			NewRoutes(NewController(nil, organization.NewRepository(db), nil, "")).Register(r.Group("/api/v1"))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/test", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)
			require.Equal(t, tc.status, resp.Code, resp.Body.String())
			require.NotContains(t, resp.Body.String(), "secret-token")
			if tc.name == "success" {
				var payload TestResponse
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
				require.Equal(t, "acme/app", payload.Data.ProjectName)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDeleteMapsRepositoryStoreErrorsToHTTPResponses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dbErr      error
		wantStatus int
	}{
		{name: "foreign key dependency", dbErr: &pgconn.PgError{Code: "23503"}, wantStatus: http.StatusConflict},
		{name: "unrelated database error", dbErr: sqlmock.ErrCancelled, wantStatus: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("repositorysettings_test_error", func(tx *gorm.DB) { tx.Error = tc.dbErr }))
			orgID, accountID := "org-a", "account-a"
			repositoryID := "11111111-1111-1111-1111-111111111111"
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2 ORDER BY "organization_memberships"."id" LIMIT $3`)).
				WithArgs(orgID, accountID, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "role"}).AddRow("membership-a", orgID, accountID, "owner"))
			controller := NewController(store.New(db).Repositories, organization.NewRepository(db), nil, "")
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/repositories/"+repositoryID, nil)
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = req
			ctx.Params = gin.Params{{Key: "repository_id", Value: repositoryID}}
			ctx.Set("organization_id", orgID)
			ctx.Set("account_id", accountID)
			controller.Delete(ctx)
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCreateMapsRepositoryIdentityConflictWithoutLeakingTenant(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dbErr      error
		wantStatus int
	}{
		{name: "repository identity conflict", dbErr: &pgconn.PgError{Code: "23505", ConstraintName: "repositories_provider_full_name_key"}, wantStatus: http.StatusConflict},
		{name: "unrelated database error", dbErr: sqlmock.ErrCancelled, wantStatus: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("repositorysettings_test_error", func(tx *gorm.DB) { tx.Error = tc.dbErr }))
			sealer, err := crypto.New([]byte("01234567890123456789012345678901"))
			require.NoError(t, err)
			controller := NewController(store.New(db).Repositories, organization.NewRepository(db), sealer, "")
			req := httptest.NewRequest(http.MethodPost, "/api/v1/repositories", bytes.NewBufferString(`{"provider":"github","full_name":"acme/app"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = req
			ctx.Set("organization_id", "org-a")
			ctx.Set("account_id", "account-a")
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2 ORDER BY "organization_memberships"."id" LIMIT $3`)).
				WithArgs("org-a", "account-a", 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "role"}).AddRow("membership-a", "org-a", "account-a", "owner"))
			controller.Create(ctx)
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			if tc.wantStatus == http.StatusConflict {
				require.NotContains(t, w.Body.String(), "org-a")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
