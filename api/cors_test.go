package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProductionCORSAllowsSessionAuthorizationPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := config.Config{CORSOrigins: []string{"https://client.example.com"}, JWTSecret: "test-secret", JWTExpiration: time.Hour}
	router.Use(productionCORS(cfg))
	registerRoutes(router, nil, nil, cfg)

	request := func(origin string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/session", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "authorization")
		router.ServeHTTP(recorder, req)
		return recorder
	}

	allowed := request("https://client.example.com")
	require.Equal(t, http.StatusNoContent, allowed.Code)
	require.Contains(t, allowed.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	disallowed := request("https://evil.example.com")
	require.NotEqual(t, "https://evil.example.com", disallowed.Header().Get("Access-Control-Allow-Origin"))
}
