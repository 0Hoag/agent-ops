package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestIDSecurityAndCORSHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), SecurityHeaders())
	router.GET("/probe", func(c *gin.Context) { c.Status(204) })
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("X-Request-ID", "req-fixture")
	router.ServeHTTP(recorder, req)
	require.Equal(t, "req-fixture", recorder.Header().Get("X-Request-ID"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
	require.Contains(t, recorder.Header().Get("Content-Security-Policy"), "default-src 'self'")
}

func TestCORSPreflightAllowsConfiguredClient(t *testing.T) {
	router := gin.New()
	router.Use(cors.New(cors.Config{AllowOrigins: []string{"http://localhost:3000"}, AllowMethods: []string{"GET", "POST", "OPTIONS"}, AllowHeaders: []string{"Content-Type", "X-Request-ID"}}))
	router.POST("/probe", func(c *gin.Context) { c.Status(204) })
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/probe", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "X-Request-ID")
	router.ServeHTTP(recorder, req)
	require.Equal(t, 204, recorder.Code)
	require.Equal(t, "http://localhost:3000", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, recorder.Header().Get("Access-Control-Allow-Methods"), "POST")
}

func TestRequestIDGeneratedWhenAbsent(t *testing.T) {
	router := gin.New()
	router.Use(RequestID())
	router.GET("/probe", func(c *gin.Context) { c.Status(204) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/probe", nil))
	require.NotEmpty(t, recorder.Header().Get("X-Request-ID"))
}
