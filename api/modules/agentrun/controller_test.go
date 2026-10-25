package agentrun

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	creates         int
	list            []AgentRun
	listFilter      Filter
	aggregates      []Aggregate
	aggregateFilter Filter
	aggregatePrices map[string]Price
}

func (f *fakeRepository) Create(_ context.Context, _ *AgentRun) error { f.creates++; return nil }
func (f *fakeRepository) List(_ context.Context, filter Filter) ([]AgentRun, error) {
	f.listFilter = filter
	return f.list, nil
}
func (f *fakeRepository) Aggregate(_ context.Context, filter Filter, prices map[string]Price) ([]Aggregate, error) {
	f.aggregateFilter = filter
	f.aggregatePrices = prices
	return f.aggregates, nil
}

func TestAggregateReturnsResultsAndForwardsFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	issue := int64(24)
	cost := 0.0125
	want := []Aggregate{{Wave: "w3a", Worker: "worker-1", IssueNumber: &issue, Runs: 2, InputTokens: 100, CachedInputTokens: 40, OutputTokens: 20, CostUSD: &cost}}
	prices := map[string]Price{"model": {InputPerMillion: 1}}
	repo := &fakeRepository{aggregates: want}
	r := gin.New()
	r.GET("/", NewController(repo, "", prices).Aggregate)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?repository_id=repo-id&wave=w3a&issue_number=24", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, Filter{RepositoryID: "repo-id", Wave: "w3a", IssueNumber: 24}, repo.aggregateFilter)
	require.Equal(t, prices, repo.aggregatePrices)
	var envelope struct {
		Data []Aggregate `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	got := envelope.Data
	require.Equal(t, want, got)
}

func TestCreateValidationPrecedesRepositoryAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	valid := CreateInput{RepositoryID: "7e57d004-2b97-0e7a-b45f-5387367791cd", Wave: "w3a", Worker: "worker", Model: "model", IssueNumbers: []int64{24}, StartedAt: now, EndedAt: now}
	tests := []struct {
		name, token string
		input       CreateInput
		want        int
	}{
		{"missing token", "", valid, http.StatusUnauthorized},
		{"wrong token", "wrong", valid, http.StatusUnauthorized},
		{"case-only changed token", "Secret", valid, http.StatusUnauthorized},
		{"negative usage", "secret", func() CreateInput { v := valid; v.InputTokens = -1; return v }(), http.StatusBadRequest},
		{"reversed timestamps", "secret", func() CreateInput { v := valid; v.EndedAt = now.Add(-time.Second); return v }(), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			r := gin.New()
			r.POST("/", NewController(repo, "secret", nil).Create)
			body, err := json.Marshal(tt.input)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				request.Header.Set("X-Agent-Run-Token", tt.token)
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			require.Equal(t, tt.want, response.Code)
			require.Zero(t, repo.creates, "invalid request must not access repository")
		})
	}
}

func TestCreatePersistsValidRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepository{}
	r := gin.New()
	r.POST("/", NewController(repo, "secret", nil).Create)
	// The fake proves the persistence operation is reached for authorized valid input.
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"repository_id":"7e57d004-2b97-0e7a-b45f-5387367791cd","wave":"w3a","worker":"worker","model":"m","issue_numbers":[24],"input_tokens":1,"cached_input_tokens":2,"cache_write_tokens":3,"output_tokens":4,"reasoning_tokens":5,"started_at":"2026-01-01T00:00:00Z","ended_at":"2026-01-01T00:00:01Z"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Agent-Run-Token", "secret")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusCreated, response.Code)
	require.Equal(t, 1, repo.creates)
}

func TestCreateRejectsEmptyConfiguredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepository{}
	r := gin.New()
	r.POST("/", NewController(repo, "", nil).Create)
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	request.Header.Set("X-Agent-Run-Token", "")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Zero(t, repo.creates)
}
