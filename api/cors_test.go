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

func TestProductionCORSAllowsOrganizationMutationPreflights(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := config.Config{CORSOrigins: []string{"https://client.example.com"}, JWTSecret: "test-secret", JWTExpiration: time.Hour}
	router.Use(productionCORS(cfg))
	registerRoutes(router, nil, nil, cfg)

	for _, test := range []struct {
		name, method, path string
	}{
		{name: "role update", method: http.MethodPatch, path: "/api/v1/organizations/org-id/members/account-id"},
		{name: "member removal", method: http.MethodDelete, path: "/api/v1/organizations/org-id/members/account-id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodOptions, test.path, nil)
			req.Header.Set("Origin", "https://client.example.com")
			req.Header.Set("Access-Control-Request-Method", test.method)
			req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusNoContent, recorder.Code)
			require.Equal(t, "https://client.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
			require.Contains(t, recorder.Header().Get("Access-Control-Allow-Methods"), test.method)
			require.Contains(t, recorder.Header().Get("Access-Control-Allow-Headers"), "Authorization")
			require.Contains(t, recorder.Header().Get("Access-Control-Allow-Headers"), "Content-Type")

			disallowed := httptest.NewRecorder()
			badOrigin := httptest.NewRequest(http.MethodOptions, test.path, nil)
			badOrigin.Header.Set("Origin", "https://evil.example.com")
			badOrigin.Header.Set("Access-Control-Request-Method", test.method)
			badOrigin.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
			router.ServeHTTP(disallowed, badOrigin)
			require.Equal(t, http.StatusForbidden, disallowed.Code)
			require.NotEqual(t, "https://evil.example.com", disallowed.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}
