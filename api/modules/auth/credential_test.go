package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/account"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestLoginCredentialResponsesAreIndistinguishable(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)

	knownHash, err := account.HashPassword("correct-password")
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	controller := NewController(NewService(account.NewRepository(database), "secret", time.Hour))
	router := gin.New()
	router.POST("/login", controller.Login)
	responses := make([]string, 0, 2)
	for _, email := range []string{"unknown@example.com", "known@example.com"} {
		rows := sqlmock.NewRows([]string{"id", "email", "password_hash"})
		if email == "known@example.com" {
			rows.AddRow("account-id", email, knownHash)
		} else {
			rows.AddRow("different-id", email, knownHash)
		}
		mock.ExpectQuery(`SELECT \* FROM "accounts"`).WithArgs(email, 1).WillReturnRows(rows)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"`+email+`","password":"wrong"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		responses = append(responses, recorder.Body.String())
	}
	require.Equal(t, responses[0], responses[1])
	require.NoError(t, mock.ExpectationsWereMet())
}
