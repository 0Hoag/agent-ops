package repositorysettings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/store"
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
			db.Callback().Delete().Before("gorm:delete").Register("repositorysettings_test_error", func(tx *gorm.DB) { tx.Error = tc.dbErr })

			orgID := "org-a"
			accountID := "account-a"
			repositoryID := "11111111-1111-1111-1111-111111111111"
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2 ORDER BY "organization_memberships"."id" LIMIT $3`)).
				WithArgs(orgID, accountID, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "role"}).AddRow("membership-a", orgID, accountID, "owner"))
			controller := NewController(store.New(db).Repositories, organization.NewRepository(db), nil, "")
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/repositories/"+repositoryID, nil)
			req.Header.Set("X-Organization-ID", orgID)
			req.Header.Set("X-Account-ID", accountID)
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
