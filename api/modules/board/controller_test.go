package board

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParsePageRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"?page=0", "?page=nope", "?per_page=0", "?per_page=101", "?per_page=nope"} {
		req := httptest.NewRequest("GET", "/api/v1/repositories"+query, nil)
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = req
		_, ok := parsePage(ctx)
		require.False(t, ok, query)
	}
}

func TestRepositoryDTOExcludesSecretReference(t *testing.T) {
	payload, err := json.Marshal(repositoryDTO(store.Repository{ID: "00000000-0000-0000-0000-000000000001", Provider: "github", FullName: "org/repo", WebhookSecretEnvRef: "SECRET_ENV"}))
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"00000000-0000-0000-0000-000000000001","provider":"github","full_name":"org/repo"}`, string(payload))
	require.NotContains(t, string(payload), "SECRET")
}

func TestBoardControllerPaginationAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := NewController(nil)
	for _, test := range []struct {
		path    string
		code    int
		message string
	}{{"/api/v1/board?repository_id=bad", 400, "invalid repository_id"}, {"/api/v1/board?repository_id=00000000-0000-0000-0000-000000000001&page=0", 400, "invalid page"}} {
		router := gin.New()
		router.GET("/api/v1/board", controller.ListBoard)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("GET", test.path, nil))
		require.Equal(t, test.code, recorder.Code)
		require.JSONEq(t, fmt.Sprintf(`{"status":400,"message":%q}`, test.message), recorder.Body.String())
	}
}

func TestIssueDTOEmptyArrays(t *testing.T) {
	encoded, err := json.Marshal(IssueDTO{Labels: json.RawMessage(`[]`), PullRequests: []PullRequestDTO{}, Number: 1})
	require.NoError(t, err)
	require.JSONEq(t, `{"labels":[],"pull_requests":[],"number":1,"id":"","repository_id":"","title":"","state":"","status":"","assignee":""}`, string(encoded))
}
