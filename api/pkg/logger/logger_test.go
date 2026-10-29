package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0Hoag/agent-ops/api/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestLogsRequestIDAndSafeRequestMetadata(t *testing.T) {
	tests := []struct {
		name       string
		suppliedID string
	}{
		{name: "supplied request id", suppliedID: "request-123"},
		{name: "generated request id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			var output bytes.Buffer
			log := newJSONLogger(&output)

			router := gin.New()
			router.Use(middleware.RequestID(), Request(log))
			router.POST("/v1/secret", func(c *gin.Context) {
				c.Status(http.StatusCreated)
			})

			req := httptest.NewRequest(http.MethodPost, "/v1/secret?token=query-secret", strings.NewReader(`{"password":"body-secret"}`))
			req.Header.Set("Authorization", "Bearer header-secret")
			req.Header.Set("Cookie", "session=cookie-secret")
			if tt.suppliedID != "" {
				req.Header.Set("X-Request-ID", tt.suppliedID)
			}
			res := httptest.NewRecorder()

			router.ServeHTTP(res, req)

			requestID := res.Header().Get("X-Request-ID")
			require.NotEmpty(t, requestID)
			if tt.suppliedID != "" {
				require.Equal(t, tt.suppliedID, requestID)
			}

			var event map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &event))
			require.Equal(t, "http request", event["msg"])
			require.Equal(t, http.MethodPost, event["method"])
			require.Equal(t, "/v1/secret", event["path"])
			require.EqualValues(t, http.StatusCreated, event["status"])
			require.GreaterOrEqual(t, event["duration_ms"], float64(0))
			require.Equal(t, requestID, event["request_id"])
			require.NotContains(t, output.String(), "header-secret")
			require.NotContains(t, output.String(), "cookie-secret")
			require.NotContains(t, output.String(), "query-secret")
			require.NotContains(t, output.String(), "body-secret")
		})
	}
}

func newJSONLogger(output *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, nil))
}
