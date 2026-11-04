package wave

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRoutesExposeAllWaveCRUDMethods(t *testing.T) {
	r := gin.New()
	NewRoutes(NewController(nil, nil)).Register(r.Group("/api/v1"))
	got := map[string]bool{}
	for _, route := range r.Routes() {
		got[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{"GET /api/v1/waves", "POST /api/v1/waves", "GET /api/v1/waves/:wave_id", "PATCH /api/v1/waves/:wave_id", "DELETE /api/v1/waves/:wave_id"} {
		require.True(t, got[route], route)
	}
}

func TestViewerCannotWriteWave(t *testing.T) {
	db, mock := waveDB(t)
	mock.ExpectQuery(`SELECT \* FROM "organization_memberships" WHERE organization_id = \$1 AND account_id = \$2`).WithArgs("org-a", "acct-a", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "role"}).AddRow("m", "org-a", "acct-a", organization.RoleViewer))
	c := NewController(NewRepository(db), organization.NewRepository(db))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/waves", func(ctx *gin.Context) {
		ctx.Set("organization_id", "org-a")
		ctx.Set("account_id", "acct-a")
		c.Create(ctx)
	})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/waves", nil))
	require.Equal(t, http.StatusForbidden, res.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
