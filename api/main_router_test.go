package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAlertHistoryProductionRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	const (
		accountID      = "11111111-1111-1111-1111-111111111111"
		organizationID = "22222222-2222-2222-2222-222222222222"
	)
	router := gin.New()
	registerRoutesWithHub(router, database.AsPinger(db), db, config.Config{
		JWTSecret:     "router-test-secret",
		JWTExpiration: time.Hour,
		EncryptionKey: []byte("01234567890123456789012345678901"),
	}, board.NewHub())

	paths := make(map[string]bool)
	for _, route := range router.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	require.True(t, paths[http.MethodGet+" /api/v1/alerts/history"], "missing production alert history route")

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/alerts/history", nil))
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": accountID, "email": "router@example.test", "organization_id": organizationID,
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("router-test-secret"))
	require.NoError(t, err)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2`)).
		WithArgs(organizationID, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "alert_occurrences" WHERE organization_id = $1`)).
		WithArgs(organizationID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`(?s).*alert_occurrences.*`).
		WithArgs(organizationID, 2, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "alert_key", "repository", "rule", "subject", "message", "detected_at"}))

	authenticated := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/history?page=2&per_page=2", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(authenticated, request)
	require.Equal(t, http.StatusOK, authenticated.Code, authenticated.Body.String())
	require.Contains(t, authenticated.Body.String(), `"total":2`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProductionRouterAuthenticatedRepositoryRoutesCoexist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	router := gin.New()
	registerRoutesWithHub(router, database.AsPinger(db), db, config.Config{
		JWTSecret:     "router-test-secret",
		JWTExpiration: time.Hour,
		EncryptionKey: []byte("01234567890123456789012345678901"),
	}, board.NewHub())

	paths := make(map[string]bool)
	for _, route := range router.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		http.MethodGet + " /api/v1/repositories",
		http.MethodPost + " /api/v1/repositories",
		http.MethodGet + " /api/v1/repositories/:repository_id",
		http.MethodPatch + " /api/v1/repositories/:repository_id",
		http.MethodDelete + " /api/v1/repositories/:repository_id",
		http.MethodGet + " /api/v1/board",
		http.MethodGet + " /api/v1/board/ws",
		http.MethodGet + " /api/v1/agent-runs/aggregate",
		http.MethodPost + " /api/v1/workers/start",
		http.MethodPost + " /api/v1/workers/restart",
		http.MethodPost + " /api/v1/workers/:worker_id/stop",
	} {
		require.Truef(t, paths[route], "missing production route %s", route)
	}

	for _, path := range []string{"/api/v1/board?repository_id=00000000-0000-0000-0000-000000000001", "/api/v1/board/ws?repository_id=00000000-0000-0000-0000-000000000001", "/api/v1/agent-runs/aggregate"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equal(t, http.StatusUnauthorized, res.Code, path)
	}
}
