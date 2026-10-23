package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	githubprovider "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
	"github.com/DATA-DOG/go-sqlmock"
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
