package alert

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHistoryIsOrganizationScopedAndPaginated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := alertDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "alert_occurrences" WHERE organization_id = $1`)).WithArgs("org-a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "alert_occurrences" WHERE organization_id = $1 ORDER BY detected_at DESC, id DESC LIMIT $2 OFFSET $3`)).WithArgs("org-a", 1, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "alert_key", "repository", "rule", "subject", "message", "detected_at"}))
	r := gin.New()
	r.GET("/history", func(c *gin.Context) { c.Set("organization_id", "org-a"); NewController(NewRepository(db)).History(c) })
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/history?page=2&per_page=1", nil))
	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), `"page":2`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHistoryRejectsUnauthenticatedContextBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := alertDB(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/history", nil)
	NewController(NewRepository(db)).History(ctx)
	require.Equal(t, http.StatusUnauthorized, ctx.Writer.Status())
	require.NoError(t, mock.ExpectationsWereMet())
}
