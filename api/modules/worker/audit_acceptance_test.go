package worker

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
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type auditAcceptanceMemberships struct{}

func (auditAcceptanceMemberships) Membership(context.Context, string, string) (*organization.Membership, error) {
	return &organization.Membership{Role: organization.RoleOwner}, nil
}

func TestAuditHistoryHTTPAcceptanceUsesMigratedTable(t *testing.T) {
	dsn := os.Getenv("W17S_AUDIT_ACCEPTANCE_DSN")
	if dsn == "" {
		t.Skip("set W17S_AUDIT_ACCEPTANCE_DSN to run disposable PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	schema := "worker_audit_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	require.NoError(t, db.Exec("SET search_path TO "+schema).Error)
	t.Cleanup(func() { _ = db.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	require.NoError(t, database.RunMigrations(context.Background(), db))

	orgA, orgB := uuid.NewString(), uuid.NewString()
	for _, org := range []string{orgA, orgB} {
		require.NoError(t, db.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", org, "audit "+org).Error)
	}
	require.NoError(t, db.Create(&ActionAudit{ID: store.UUID(uuid.NewString()), OrganizationID: orgA, ActorID: "actor-a", Action: "start", Wave: "W17", JobID: "job-a", Outcome: "succeeded"}).Error)
	require.NoError(t, db.Create(&ActionAudit{ID: store.UUID(uuid.NewString()), OrganizationID: orgB, ActorID: "actor-b", Action: "stop", Wave: "W17", JobID: "job-b", Outcome: "succeeded"}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("organization_id", c.GetHeader("X-Organization-ID"))
		c.Set("account_id", "owner")
		c.Next()
	})
	NewRoutes(NewController(nil, auditAcceptanceMemberships{}, NewAuditRepository(db))).Register(r.Group("/api/v1"))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workers/audit-history", nil)
	request.Header.Set("X-Organization-ID", orgA)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var got AuditHistoryResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &got))
	require.Len(t, got.Data, 1)
	require.Equal(t, "actor-a", got.Data[0].ActorID)
	require.Equal(t, int64(1), got.Meta.Total)
}
