package repositorysettings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const acceptanceDSN = "W8D_REPOSITORY_ACCEPTANCE_DSN"

func TestRepositorySettingsHTTPAcceptance(t *testing.T) {
	dsn := os.Getenv(acceptanceDSN)
	if dsn == "" {
		t.Skip("set " + acceptanceDSN + " to run disposable PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	schema := "repositorysettings_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	require.NoError(t, db.Exec("SET search_path TO "+schema).Error)
	t.Cleanup(func() { _ = db.Exec("DROP SCHEMA " + schema + " CASCADE").Error; _ = sqlDB.Close() })
	require.NoError(t, database.RunMigrations(context.Background(), db))
	fixture := seedSettingsFixture(t, db)
	key := []byte("01234567890123456789012345678901")
	sealer, err := crypto.New(key)
	require.NoError(t, err)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("organization_id", c.GetHeader("X-Organization-ID"))
		c.Set("account_id", c.GetHeader("X-Account-ID"))
		c.Next()
	})
	NewRoutes(NewController(store.New(db).Repositories, organization.NewRepository(db), sealer, "https://agent.example")).Register(r.Group("/api/v1"))
	h := http.Handler(r)
	create := request(t, h, fixture.ownerA, fixture.orgA, http.MethodPost, "/api/v1/repositories", `{"provider":"github","full_name":"acme/a","webhook_secret":"secret-a","backfill_token":"token-a","status_mappings":{"status:review":"review"}}`)
	require.Equal(t, http.StatusCreated, create.Code, create.Body.String())
	var created CreatedResponse
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &created))
	require.Equal(t, "secret-a", created.Data.WebhookSecret)
	require.Equal(t, "token-a", created.Data.BackfillToken)
	var row store.Repository
	require.NoError(t, db.First(&row, "id = ?", created.Data.ID).Error)
	require.NotEqual(t, "secret-a", row.WebhookSecretCiphertext)
	require.NotEqual(t, "token-a", row.BackfillTokenCiphertext)
	opened, err := sealer.Open(row.WebhookSecretCiphertext)
	require.NoError(t, err)
	require.Equal(t, "secret-a", opened)
	opened, err = sealer.Open(row.BackfillTokenCiphertext)
	require.NoError(t, err)
	require.Equal(t, "token-a", opened)

	for _, tc := range []struct {
		name, actor, org string
		method           string
		path             string
		want             int
	}{
		{"owner list", fixture.ownerA, fixture.orgA, http.MethodGet, "/api/v1/repositories", 200},
		{"admin get", fixture.adminA, fixture.orgA, http.MethodGet, "/api/v1/repositories/" + created.Data.ID, 200},
		{"member update denied", fixture.memberA, fixture.orgA, http.MethodPatch, "/api/v1/repositories/" + created.Data.ID, 403},
		{"viewer delete denied", fixture.viewerA, fixture.orgA, http.MethodDelete, "/api/v1/repositories/" + created.Data.ID, 403},
		{"other tenant get missing", fixture.ownerB, fixture.orgB, http.MethodGet, "/api/v1/repositories/" + created.Data.ID, 404},
		{"other tenant patch missing", fixture.ownerB, fixture.orgB, http.MethodPatch, "/api/v1/repositories/" + created.Data.ID, 404},
		{"other tenant delete missing", fixture.ownerB, fixture.orgB, http.MethodDelete, "/api/v1/repositories/" + created.Data.ID, 404},
	} {
		w := request(t, h, tc.actor, tc.org, tc.method, tc.path, `{"webhook_secret":"not-authorized"}`)
		require.Equal(t, tc.want, w.Code, tc.name+": "+w.Body.String())
	}
	for _, actor := range []string{fixture.ownerA, fixture.adminA} {
		w := request(t, h, actor, fixture.orgA, http.MethodGet, "/api/v1/repositories/"+created.Data.ID, "")
		require.Equal(t, 200, w.Code)
		require.NotContains(t, w.Body.String(), "secret-a")
		require.NotContains(t, w.Body.String(), "token-a")
		require.NotContains(t, w.Body.String(), "webhook_secret")
		require.NotContains(t, w.Body.String(), "backfill_token")
	}
	list := request(t, h, fixture.ownerA, fixture.orgA, http.MethodGet, "/api/v1/repositories", "")
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	require.NotContains(t, list.Body.String(), "secret-a")
	require.NotContains(t, list.Body.String(), "token-a")
	require.NotContains(t, list.Body.String(), "webhook_secret")
	require.NotContains(t, list.Body.String(), "backfill_token")
	beforeCrossTenant := row
	crossTenant := request(t, h, fixture.ownerB, fixture.orgB, http.MethodPatch, "/api/v1/repositories/"+created.Data.ID, `{"webhook_secret":"cross-tenant","backfill_token":"cross-token"}`)
	require.Equal(t, http.StatusNotFound, crossTenant.Code, crossTenant.Body.String())
	require.NoError(t, db.First(&row, "id = ?", created.Data.ID).Error)
	require.Equal(t, beforeCrossTenant.WebhookSecretCiphertext, row.WebhookSecretCiphertext)
	require.Equal(t, beforeCrossTenant.BackfillTokenCiphertext, row.BackfillTokenCiphertext)
	updatedSecret, updatedToken := "secret-updated", "token-updated"
	w := request(t, h, fixture.adminA, fixture.orgA, http.MethodPatch, "/api/v1/repositories/"+created.Data.ID, `{"webhook_secret":"`+updatedSecret+`","backfill_token":"`+updatedToken+`"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), updatedSecret)
	require.NotContains(t, w.Body.String(), updatedToken)
	require.NotContains(t, w.Body.String(), "webhook_secret")
	require.NotContains(t, w.Body.String(), "backfill_token")
	require.NoError(t, db.First(&row, "id = ?", created.Data.ID).Error)
	opened, err = sealer.Open(row.WebhookSecretCiphertext)
	require.NoError(t, err)
	require.Equal(t, updatedSecret, opened)
	opened, err = sealer.Open(row.BackfillTokenCiphertext)
	require.NoError(t, err)
	require.Equal(t, updatedToken, opened)
	require.Equal(t, 204, request(t, h, fixture.adminA, fixture.orgA, http.MethodDelete, "/api/v1/repositories/"+created.Data.ID, "").Code)
	require.Equal(t, 404, request(t, h, fixture.ownerA, fixture.orgA, http.MethodGet, "/api/v1/repositories/"+created.Data.ID, "").Code)
}

type settingsFixture struct{ orgA, orgB, ownerA, adminA, memberA, viewerA, ownerB string }

func seedSettingsFixture(t *testing.T, db *gorm.DB) settingsFixture {
	f := settingsFixture{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for _, account := range []string{f.ownerA, f.adminA, f.memberA, f.viewerA, f.ownerB} {
		require.NoError(t, db.Exec("INSERT INTO accounts (id,email,password_hash) VALUES (?,?,?)", account, account+"@example.test", "fixture").Error)
	}
	for _, org := range []string{f.orgA, f.orgB} {
		require.NoError(t, db.Exec("INSERT INTO organizations (id,name) VALUES (?,?)", org, org).Error)
	}
	for _, m := range []struct{ o, a, r string }{{f.orgA, f.ownerA, "owner"}, {f.orgA, f.adminA, "admin"}, {f.orgA, f.memberA, "member"}, {f.orgA, f.viewerA, "viewer"}, {f.orgB, f.ownerB, "owner"}} {
		require.NoError(t, db.Exec("INSERT INTO organization_memberships (id,organization_id,account_id,role) VALUES (?,?,?,?)", uuid.NewString(), m.o, m.a, m.r).Error)
	}
	return f
}
func request(t *testing.T, h http.Handler, account, org, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Account-ID", account)
	req.Header.Set("X-Organization-ID", org)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
