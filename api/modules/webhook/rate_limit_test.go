package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRateLimiterRefillsAndIsolatesRepositories(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter(2, time.Second, 8, func() time.Time { return clock })
	repoA, repoB := store.UUID("a"), store.UUID("b")
	require.True(t, limiter.Allow(repoA))
	require.True(t, limiter.Allow(repoA))
	require.False(t, limiter.Allow(repoA))
	require.True(t, limiter.Allow(repoB), "a repository must not consume another repository's quota")
	clock = clock.Add(500 * time.Millisecond)
	require.True(t, limiter.Allow(repoA), "a half-second should refill one token")
}

func TestRateLimiterConcurrentAccessIsBounded(t *testing.T) {
	limiter := NewRateLimiter(1, time.Hour, 2, time.Now)
	var wg sync.WaitGroup
	allowed := make(chan bool, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); allowed <- limiter.Allow("repo") }()
	}
	wg.Wait()
	close(allowed)
	count := 0
	for value := range allowed {
		if value {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestWebhookRouteReturns429BeforePersistenceWhenRepositoryBucketIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock, db := newWebhookMockDB(t)
	service := NewService(db, fakeRegistry{})
	service.limiter = NewRateLimiter(1, time.Hour, 8, time.Now)
	router := gin.New()
	NewRoutes(NewController(service, map[string][]Config{"github": {{Repository: "org/repo", SecretEnv: "W16G_SECRET"}}})).Register(router.Group("/api/v1"))
	payload := []byte(`{"repository":{"full_name":"org/repo"},"issue":{"number":1,"title":"issue","state":"open"}}`)
	setup := func(delivery string) {
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT \* FROM "repositories"`).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "created_at", "updated_at"}).AddRow("11111111-1111-1111-1111-111111111111", "github", "org/repo", "SECRET", nil, nil))
		if delivery == "first" {
			mock.ExpectExec(`INSERT INTO "webhook_events"`).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(`INSERT INTO "issues"`).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
		} else {
			mock.ExpectRollback()
		}
	}
	t.Setenv("W16G_SECRET", "unused")
	for _, delivery := range []string{"first", "second"} {
		setup(delivery)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(string(payload)))
		req.Header.Set("X-GitHub-Delivery", delivery)
		req.Header.Set("X-GitHub-Event", "issues")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if delivery == "first" {
			require.Equal(t, http.StatusOK, res.Code)
		} else {
			require.Equal(t, http.StatusTooManyRequests, res.Code)
		}
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInvalidSignatureDoesNotConsumeRepositoryQuota(t *testing.T) {
	limiter := NewRateLimiter(1, time.Hour, 8, time.Now)
	service := NewService(nil, configuredRegistry{})
	service.limiter = limiter
	body := []byte(`{"repository":{"full_name":"org/repo"},"issue":{"number":1,"title":"issue","state":"open"}}`)
	h := http.Header{"X-GitHub-Delivery": []string{"invalid"}, "X-GitHub-Event": []string{"issues"}, "X-Hub-Signature-256": []string{"sha256=" + hex.EncodeToString(make([]byte, sha256.Size))}}
	_, err := service.Ingest(context.Background(), "github", "org/repo", "secret", "", "", h, body)
	require.ErrorIs(t, err, ErrRejected)
	require.True(t, limiter.Allow("11111111-1111-1111-1111-111111111111"))
}
