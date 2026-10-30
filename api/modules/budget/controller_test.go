package budget

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCreateRejectsNonMemberBeforeBudgetWrite(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT \* FROM "organization_memberships" WHERE organization_id = \$1 AND account_id = \$2`).
		WithArgs("org-a", "account-a", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("organization_id", "org-a"); c.Set("account_id", "account-a"); c.Next() })
	NewRoutes(NewController(NewRepository(db), organization.NewRepository(db))).Register(r.Group(""))
	req := httptest.NewRequest(http.MethodPost, "/budgets", bytes.NewBufferString(`{"token_limit":100,"starts_at":"2026-01-01T00:00:00Z","ends_at":"2026-01-02T00:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusForbidden, resp.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
