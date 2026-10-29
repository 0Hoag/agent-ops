package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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
		http.MethodGet + " /metrics",
		http.MethodGet + " /api/v1/repositories",
		http.MethodPost + " /api/v1/repositories",
		http.MethodGet + " /api/v1/repositories/:repository_id",
		http.MethodPatch + " /api/v1/repositories/:repository_id",
		http.MethodDelete + " /api/v1/repositories/:repository_id",
		http.MethodGet + " /api/v1/board",
		http.MethodGet + " /api/v1/board/ws",
		http.MethodGet + " /api/v1/agent-runs/aggregate",
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
