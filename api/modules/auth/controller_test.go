package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSessionRequiresBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := NewController(NewService(nil, "secret", time.Hour))
	router := gin.New()
	router.GET("/session", controller.Session)
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer abc extra"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/session", nil)
		request.Header.Set("Authorization", header)
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
	}
}

func TestLoginRejectsMalformedCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := NewController(NewService(nil, "secret", time.Hour))
	router := gin.New()
	router.POST("/login", controller.Login)
	for _, body := range []string{"{", `{"email":" ","password":"x"}`, `{"email":"a@b.c","password":""}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	}
}
