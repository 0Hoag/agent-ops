package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestRequireSessionAuthenticatesBrowserWebSocketToken(t *testing.T) {
	token := signedSessionToken(t, jwt.MapClaims{
		"sub": "account-id", "email": "person@example.com", "organization_id": "org-id",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	service := auth.NewService(nil, "secret", time.Hour)
	router := gin.New()
	router.Use(RequireSession(service))
	router.GET("/logs", func(c *gin.Context) {
		session, ok := Session(c)
		require.True(t, ok)
		require.Equal(t, "org-id", session.OrganizationID)
		c.Status(101)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/logs?access_token="+token, nil)
	request.Header.Set("Connection", "keep-alive, Upgrade")
	request.Header.Set("Upgrade", "WebSocket")
	router.ServeHTTP(recorder, request)

	require.Equal(t, 101, recorder.Code)
	require.NotContains(t, recorder.Body.String(), token)
}

func TestRequireSessionRejectsInvalidOrExpiredWebSocketTokens(t *testing.T) {
	service := auth.NewService(nil, "secret", time.Hour)
	for _, test := range []struct {
		name  string
		token string
	}{
		{name: "malformed", token: "not-a-jwt"},
		{name: "expired", token: signedSessionToken(t, jwt.MapClaims{
			"sub": "account-id", "email": "person@example.com", "organization_id": "org-id",
			"exp": time.Now().Add(-time.Minute).Unix(),
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RequireSession(service))
			router.GET("/logs", func(c *gin.Context) { t.Fatal("invalid session reached handler") })
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/logs?access_token="+test.token, nil)
			request.Header.Set("Connection", "Upgrade")
			request.Header.Set("Upgrade", "websocket")
			router.ServeHTTP(recorder, request)
			require.Equal(t, 401, recorder.Code)
			require.NotContains(t, recorder.Body.String(), test.token)
		})
	}
}

func TestRequireSessionRejectsBrowserTokenOutsideWebSocketUpgrade(t *testing.T) {
	token := signedSessionToken(t, jwt.MapClaims{
		"sub": "account-id", "email": "person@example.com", "organization_id": "org-id",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	service := auth.NewService(nil, "secret", time.Hour)
	router := gin.New()
	router.Use(RequireSession(service))
	router.GET("/logs", func(c *gin.Context) { t.Fatal("alternate credential reached REST handler") })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/logs?access_token="+token, nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, 401, recorder.Code)
}

func TestRequireSessionRetainsBearerAuthentication(t *testing.T) {
	token := signedSessionToken(t, jwt.MapClaims{
		"sub": "account-id", "email": "person@example.com", "organization_id": "org-id",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	service := auth.NewService(nil, "secret", time.Hour)
	router := gin.New()
	router.Use(RequireSession(service))
	router.GET("/logs", func(c *gin.Context) { c.Status(204) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/logs", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, request)
	require.Equal(t, 204, recorder.Code)
}

func signedSessionToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	value, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	require.NoError(t, err)
	return value
}
