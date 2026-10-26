package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/board"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	githubprovider "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestValidEntity(t *testing.T) {
	cases := []struct {
		name  string
		event scm.Event
		want  bool
	}{
		{"issue", scm.Event{Issue: &scm.Issue{Number: 1, Title: "t", State: "open"}}, true},
		{"issue zero", scm.Event{Issue: &scm.Issue{Title: "t", State: "open"}}, false},
		{"issue state empty", scm.Event{Issue: &scm.Issue{Number: 1, Title: "t"}}, false},
		{"pull", scm.Event{PullRequest: &scm.PullRequest{Number: 1, Title: "t", State: "open", HeadSHA: "sha", HeadBranch: "main"}}, true},
		{"pull sha empty", scm.Event{PullRequest: &scm.PullRequest{Number: 1, Title: "t", State: "open", HeadBranch: "main"}}, false},
		{"run", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "sha", Status: "completed", StartedAt: time.Now()}}, true},
		{"run zero", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "0", SHA: "sha", Status: "completed", StartedAt: time.Now()}}, false},
		{"run status empty", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "sha", StartedAt: time.Now()}}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := validEntity(test.event); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestEventTypeUsesProviderHeader(t *testing.T) {
	h := http.Header{"X-GitHub-Event": []string{"issues"}, "X-Gitlab-Event": []string{"Issue Hook"}}
	require.Equal(t, "issues", eventType("github", h))
	require.Equal(t, "Issue Hook", eventType("gitlab", h))
}

type fakeProvider struct{}

func (fakeProvider) VerifyWebhook(context.Context, http.Header, []byte) error { return nil }
func (fakeProvider) ParseEvent(_ context.Context, kind string, h http.Header, payload []byte) (scm.Event, error) {
	var b struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Issue struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(payload, &b); err != nil {
		return scm.Event{}, err
	}
	if b.Repository.FullName == "" {
		return scm.Event{}, errors.New("missing repository")
	}
	return scm.Event{Repository: b.Repository.FullName, DeliveryID: h.Get("X-GitHub-Delivery"), EventType: kind, Issue: &scm.Issue{Number: b.Issue.Number, Title: b.Issue.Title, State: b.Issue.State}}, nil
}
func (fakeProvider) Backfill(context.Context, string) (scm.BackfillResult, error) {
	return scm.BackfillResult{}, nil
}

type fakeRegistry struct{}

func (fakeRegistry) Build(string, scm.Config) (scm.SCMProvider, error) { return fakeProvider{}, nil }

type badRegistry struct{}

func (badRegistry) Build(string, scm.Config) (scm.SCMProvider, error) {
	return nil, errors.New("internal secret detail")
}

type configuredRegistry struct{}

func (configuredRegistry) Build(name string, config scm.Config) (scm.SCMProvider, error) {
	if name != "github" {
		return nil, errors.New("unknown provider")
	}
	return githubprovider.New(config), nil
}

func TestIngestRejectsRepoMismatchAndWrongSecret(t *testing.T) {
	s := NewService(nil, configuredRegistry{})
	body := []byte(`{"repository":{"full_name":"org/a"},"issue":{"id":101,"number":1,"title":"t","state":"open"}}`)
	h := http.Header{"X-GitHub-Delivery": []string{"d1"}, "X-GitHub-Event": []string{"issues"}}
	_, err := s.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", h, body)
	if err != ErrRejected {
		t.Fatalf("mismatch err=%v", err)
	}
	mac := hmac.New(sha256.New, []byte("secret-a"))
	_, _ = mac.Write(body)
	h.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	_, err = s.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", h, body)
	if err != ErrRejected {
		t.Fatalf("signature err=%v", err)
	}
}

func TestIngestWithRealGitHubProviderAcceptsValidSignature(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"repository":{"full_name":"org/a"},"issue":{"id":101,"number":1,"title":"t","state":"open"}}`)
	h := http.Header{"X-GitHub-Delivery": []string{"valid-1"}, "X-GitHub-Event": []string{"issues"}}
	mac := hmac.New(sha256.New, []byte("secret-a"))
	_, _ = mac.Write(body)
	h.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if _, err := githubprovider.New(scm.Config{WebhookSecret: "secret-a"}).ParseEvent(context.Background(), "issues", h, body); err != nil {
		t.Fatalf("provider parse=%v", err)
	}
	service := NewService(db, configuredRegistry{})
	provider, err := service.VerifyConfigured(context.Background(), "github", "secret-a", "", "", h, body)
	if err != nil {
		t.Fatalf("verify configured: %v", err)
	}
	if _, err := provider.ParseEvent(context.Background(), "issues", h, body); err != nil {
		t.Fatalf("verified provider rejected valid event: %v", err)
	}
}

func TestVerifyConfiguredForRepositoryUsesPersistedSecretAndLegacyFallback(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	sealer, err := appcrypto.New([]byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	ciphertext, err := sealer.Seal("persisted-secret")
	require.NoError(t, err)
	body := []byte(`{"repository":{"full_name":"org/a"},"issue":{"id":101,"number":1,"title":"t","state":"open"}}`)
	h := http.Header{"X-GitHub-Delivery": []string{"persisted-1"}, "X-GitHub-Event": []string{"issues"}}
	mac := hmac.New(sha256.New, []byte("persisted-secret"))
	_, _ = mac.Write(body)
	h.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).
		WithArgs("github", "org/a", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "webhook_secret_ciphertext", "backfill_token_ciphertext", "git_lab_base_url", "status_mappings", "organization_id", "created_at", "updated_at"}).
			AddRow("11111111-1111-1111-1111-111111111111", "github", "org/a", "LEGACY_SECRET", ciphertext, "", "", []byte(`{}`), "org", nil, nil))
	service := NewServiceWithPublisherAndSealer(db, configuredRegistry{}, nil, sealer)
	_, err = service.VerifyConfiguredForRepository(context.Background(), "github", "org/a", "wrong-env-secret", "", "", h, body)
	require.NoError(t, err, "persisted ciphertext must override legacy env-ref secret")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestControllerHandleBindsSignedPayloadToConfiguredRepository(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		repositoryA   = "org/a"
		repositoryB   = "org/b"
		secretA       = "secret-a"
		secretB       = "secret-b"
		repositoryAID = "11111111-1111-1111-1111-111111111111"
	)
	payloadFor := func(repository string) []byte {
		return []byte(fmt.Sprintf(`{"repository":{"full_name":%q},"issue":{"id":101,"number":1,"title":"signed issue","state":"open","labels":[{"name":"status:review"}]}}`, repository))
	}
	sign := func(payload []byte, secret string) string {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(payload)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	t.Run("matching secret persists raw and normalized identity", func(t *testing.T) {
		mock, db := newWebhookMockDB(t)
		payload := payloadFor(repositoryA)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).WithArgs("github", repositoryA, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "created_at", "updated_at"}).AddRow(repositoryAID, "github", repositoryA, "GITHUB_A_SECRET", nil, nil))
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WithArgs(sqlmock.AnyArg(), repositoryAID, "github", "issues", "delivery-a", payloadContains("org/a"), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "issues"`)).WithArgs(sqlmock.AnyArg(), repositoryAID, int64(1), "signed issue", "open", "review", sqlmock.AnyArg(), "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		response := postWebhook(t, db, map[string][]Config{"github": {{Repository: repositoryA, SecretEnv: "W1CR7_REPO_A_SECRET"}, {Repository: repositoryB, SecretEnv: "W1CR7_REPO_B_SECRET"}}}, payload, "delivery-a", sign(payload, secretA))
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s, want 200", response.Code, response.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("secret for another repository rejects without writes", func(t *testing.T) {
		mock, db := newWebhookMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("(?!)")
		payload := payloadFor(repositoryA)
		response := postWebhook(t, db, map[string][]Config{"github": {{Repository: repositoryA, SecretEnv: "W1CR7_REPO_A_SECRET"}, {Repository: repositoryB, SecretEnv: "W1CR7_REPO_B_SECRET"}}}, payload, "delivery-b", sign(payload, secretB))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s, want 400", response.Code, response.Body.String())
		}
		assertNoDatabaseAccess(t, mock)
	})
	t.Run("valid signature for unknown repository rejects without writes", func(t *testing.T) {
		mock, db := newWebhookMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("(?!)")
		payload := payloadFor("org/unknown")
		response := postWebhook(t, db, map[string][]Config{"github": {{Repository: repositoryA, SecretEnv: "W1CR7_REPO_A_SECRET"}, {Repository: repositoryB, SecretEnv: "W1CR7_REPO_B_SECRET"}}}, payload, "delivery-unknown", sign(payload, secretA))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s, want 400", response.Code, response.Body.String())
		}
		assertNoDatabaseAccess(t, mock)
	})
}

func TestWebhookRoutePublishesWebSocketInvalidationOnlyAfterCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		repositoryA   = "org/a"
		repositoryB   = "org/b"
		repositoryAID = "11111111-1111-1111-1111-111111111111"
		repositoryBID = "22222222-2222-2222-2222-222222222222"
	)
	payload := []byte(`{"repository":{"full_name":"org/a"},"issue":{"id":101,"number":1,"title":"signed issue","state":"open"}}`)
	sign := func(body []byte) string {
		mac := hmac.New(sha256.New, []byte("secret-a"))
		_, _ = mac.Write(body)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	for _, tc := range []struct {
		name           string
		setup          func(sqlmock.Sqlmock)
		delivery       string
		validSignature bool
		wantEvent      bool
	}{
		{name: "commit", setup: func(mock sqlmock.Sqlmock) {
			expectWebhookCommit(mock, repositoryA, repositoryAID, "delivery-commit", true)
		}, delivery: "delivery-commit", validSignature: true, wantEvent: true},
		{name: "duplicate", setup: func(mock sqlmock.Sqlmock) {
			expectWebhookCommit(mock, repositoryA, repositoryAID, "delivery-duplicate", false)
		}, delivery: "delivery-duplicate", validSignature: true},
		{name: "rollback", setup: func(mock sqlmock.Sqlmock) {
			expectWebhookRollback(mock, repositoryA, repositoryAID, "delivery-rollback")
		}, delivery: "delivery-rollback", validSignature: true},
		{name: "failed commit", setup: func(mock sqlmock.Sqlmock) {
			expectWebhookFailedCommit(mock, repositoryA, repositoryAID, "delivery-failed-commit")
		}, delivery: "delivery-failed-commit", validSignature: true},
		{name: "rejected signature", delivery: "delivery-rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock, db := newWebhookMockDB(t)
			hub := board.NewHub()
			router := gin.New()
			NewRoutes(NewController(NewServiceWithPublisher(db, configuredRegistry{}, hub), map[string][]Config{"github": {{Repository: repositoryA, SecretEnv: "W2C_A_SECRET"}, {Repository: repositoryB, SecretEnv: "W2C_B_SECRET"}}})).Register(router.Group("/api/v1"))
			board.NewRoutes(board.NewControllerWithHub(nil, hub)).Register(router.Group("/api/v1"))
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			connect := func(id string) *websocket.Conn {
				conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"/api/v1/board/ws?repository_id="+id, nil)
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })
				return conn
			}
			connA, connB := connect(repositoryAID), connect(repositoryBID)
			awaitSubscribers(t, hub, map[string]int{repositoryAID: 1, repositoryBID: 1})
			if tc.setup != nil {
				tc.setup(mock)
			}
			t.Setenv("W2C_A_SECRET", "secret-a")
			t.Setenv("W2C_B_SECRET", "secret-b")
			request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(string(payload)))
			request.Header.Set("X-GitHub-Delivery", tc.delivery)
			request.Header.Set("X-GitHub-Event", "issues")
			if tc.validSignature {
				request.Header.Set("X-Hub-Signature-256", sign(payload))
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if tc.wantEvent {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				_ = connA.SetReadDeadline(time.Now().Add(time.Second))
				_, message, err := connA.ReadMessage()
				require.NoError(t, err)
				require.JSONEq(t, `{"type":"board.invalidate","repository_id":"`+repositoryAID+`"}`, string(message))
			} else {
				if tc.name == "duplicate" {
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), `"accepted":false`)
				} else if tc.validSignature {
					require.NotEqual(t, http.StatusOK, response.Code, response.Body.String())
				} else {
					require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				}
			}
			for label, conn := range map[string]*websocket.Conn{"repository A": connA, "repository B": connB} {
				if label == "repository A" && tc.wantEvent {
					continue
				}
				_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				_, _, err := conn.ReadMessage()
				require.Error(t, err, "%s unexpectedly received an event", label)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func awaitSubscribers(t *testing.T, hub *board.Hub, expected map[string]int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		ready := true
		for repositoryID, count := range expected {
			if hub.SubscriberCount(repositoryID) != count {
				ready = false
				break
			}
		}
		if ready {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("websocket subscribers did not become ready: expected %v", expected)
		}
		runtime.Gosched()
	}
}

func expectWebhookCommit(mock sqlmock.Sqlmock, name, id, delivery string, created bool) {
	expectWebhookTransactionStart(mock, name, id, delivery)
	rows := int64(1)
	if !created {
		rows = 0
	}
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, rows))
	if created {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "issues"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectCommit()
}

func expectWebhookRollback(mock sqlmock.Sqlmock, name, id, delivery string) {
	expectWebhookTransactionStart(mock, name, id, delivery)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "issues"`)).WillReturnError(errors.New("issue write failed"))
	mock.ExpectRollback()
}

func expectWebhookFailedCommit(mock sqlmock.Sqlmock, name, id, delivery string) {
	expectWebhookTransactionStart(mock, name, id, delivery)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "issues"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
}

func expectWebhookTransactionStart(mock sqlmock.Sqlmock, name, id, delivery string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).WithArgs("github", name, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "created_at", "updated_at"}).AddRow(id, "github", name, "SECRET", nil, nil))
	// Entity parsing uses the real GitHub provider and identity; the persisted JSON is asserted by the model tests.
	_ = delivery
}

func assertNoDatabaseAccess(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	err := mock.ExpectationsWereMet()
	if err == nil {
		t.Fatal("unexpected database access: all database expectations were consumed")
	}
	diagnostic := err.Error()
	if strings.Contains(diagnostic, "ExpectedQuery") || !strings.Contains(diagnostic, "ExpectedBegin") {
		t.Fatalf("unexpected database access diagnostic missing: %v", err)
	}
}

func newWebhookMockDB(t *testing.T) (sqlmock.Sqlmock, *gorm.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return mock, db
}

type payloadContains string

func (expected payloadContains) Match(value driver.Value) bool {
	payload, ok := value.([]byte)
	return ok && strings.Contains(string(payload), string(expected))
}

func postWebhook(t *testing.T, db *gorm.DB, configs map[string][]Config, payload []byte, delivery, signature string) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("W1CR7_REPO_A_SECRET", "secret-a")
	t.Setenv("W1CR7_REPO_B_SECRET", "secret-b")
	service := NewService(db, configuredRegistry{})
	router := gin.New()
	NewRoutes(NewController(service, configs)).Register(router.Group("/api/v1"))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(string(payload)))
	request.Header.Set("X-GitHub-Delivery", delivery)
	request.Header.Set("X-GitHub-Event", "issues")
	request.Header.Set("X-Hub-Signature-256", signature)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestIngestWithRealGitHubProviderRejectsInvalidSignatureWithoutWrites(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"repository":{"full_name":"org/a"},"issue":{"id":101,"number":1,"title":"t","state":"open"}}`)
	h := http.Header{"X-GitHub-Delivery": []string{"invalid-1"}, "X-GitHub-Event": []string{"issues"}, "X-Hub-Signature-256": []string{"sha256=" + hex.EncodeToString(make([]byte, sha256.Size))}}
	service := NewService(db, configuredRegistry{})
	if _, err := service.Ingest(context.Background(), "github", "org/a", "secret-a", "", "", h, body); err != ErrRejected {
		t.Fatalf("invalid signature err=%v, want rejection", err)
	}
}

func TestUnknownProviderReturnsSafeError(t *testing.T) {
	_, err := NewService(nil, badRegistry{}).Ingest(context.Background(), "unknown", "repo", "", "", "", http.Header{}, nil)
	if err != ErrRejected {
		t.Fatalf("err=%v", err)
	}
	if err.Error() == "internal secret detail" {
		t.Fatal("internal details leaked")
	}
}

func TestPersistEventRollsBackDeliveryWhenEntityUpsertFailsAndRetries(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, fakeRegistry{})
	repoID := "11111111-1111-1111-1111-111111111111"
	repoRows := sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "created_at", "updated_at"}).AddRow(repoID, "github", "org/repo", "SECRET", nil, nil)
	event := scm.Event{Repository: "org/repo", DeliveryID: "delivery-retry", EventType: "issues", ReceivedAt: time.Now().UTC(), Issue: &scm.Issue{Number: 7, Title: "retry me", State: "open"}}

	// The first attempt must roll back both the raw delivery and the failed entity upsert.
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).WithArgs("github", "org/repo", 1).WillReturnRows(repoRows)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "issues"`)).WillReturnError(errors.New("entity write failed"))
	mock.ExpectRollback()
	created, err := s.persistEvent(context.Background(), "github", "org/repo", event, nil)
	if err == nil || created {
		t.Fatalf("first attempt created=%v err=%v, want rollback error", created, err)
	}

	// The same delivery is allowed to succeed after the transient entity fault is removed.
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).WithArgs("github", "org/repo", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "created_at", "updated_at"}).AddRow(repoID, "github", "org/repo", "SECRET", nil, nil))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "issues"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	created, err = s.persistEvent(context.Background(), "github", "org/repo", event, nil)
	if err != nil || !created {
		t.Fatalf("retry created=%v err=%v, want success", created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistEventTreatsDuplicateDeliveryAsNoOp(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, fakeRegistry{})
	event := scm.Event{Repository: "org/repo", DeliveryID: "delivery-once", EventType: "issues", ReceivedAt: time.Now().UTC(), Issue: &scm.Issue{Number: 8, Title: "once", State: "open"}}
	for i := 0; i < 2; i++ {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).WithArgs("github", "org/repo", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "created_at", "updated_at"}).AddRow("11111111-1111-1111-1111-111111111111", "github", "org/repo", "SECRET", nil, nil))
		rows := int64(1)
		if i == 1 {
			rows = 0
		}
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, rows))
		if i == 0 {
			mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "issues"`)).WillReturnResult(sqlmock.NewResult(1, 1))
		}
		mock.ExpectCommit()
		created, err := s.persistEvent(context.Background(), "github", "org/repo", event, nil)
		if err != nil || created != (i == 0) {
			t.Fatalf("attempt %d created=%v err=%v", i+1, created, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
