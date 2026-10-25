package handlers

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakePinger struct{ err error }

func (f fakePinger) PingContext(context.Context) error { return f.err }

func TestHealthzIsIndependentFromDatabase(t *testing.T) {
	router := gin.New()
	NewHealth(fakePinger{err: errors.New("database down")}).Register(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/healthz", nil))
	require.Equal(t, 200, recorder.Code)
	require.JSONEq(t, `{"status":200,"data":{"status":"ok"}}`, recorder.Body.String())
}

func TestReadyzReportsDatabaseState(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "ready", code: 200},
		{name: "database down", err: errors.New("connection refused"), code: 503},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			NewHealth(fakePinger{err: test.err}).Register(router)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("GET", "/readyz", nil))
			require.Equal(t, test.code, recorder.Code)
		})
	}
}
