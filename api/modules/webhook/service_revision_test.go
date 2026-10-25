package webhook

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestPersistEventBindsNonZeroRevisionForEachEntity(t *testing.T) {
	updatedAt := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		event scm.Event
		query string
		args  []driver.Value
	}{
		{"issue", scm.Event{Issue: &scm.Issue{Number: 1, Title: "issue", State: "open", UpdatedAt: updatedAt}}, `INSERT INTO "issues"`, []driver.Value{sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1), "issue", "open", "", []byte("null"), "", updatedAt, sqlmock.AnyArg(), sqlmock.AnyArg()}},
		{"pull request", scm.Event{PullRequest: &scm.PullRequest{Number: 2, Title: "pull", State: "open", HeadSHA: "sha", HeadBranch: "main", UpdatedAt: updatedAt}}, `INSERT INTO "pull_requests"`, []driver.Value{sqlmock.AnyArg(), sqlmock.AnyArg(), int64(2), "pull", "open", "sha", "main", []byte("null"), sqlmock.AnyArg(), updatedAt, sqlmock.AnyArg(), sqlmock.AnyArg()}},
		{"pipeline", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "run-3", SHA: "sha", Status: "completed", StartedAt: updatedAt.Add(-time.Hour), UpdatedAt: updatedAt}}, `INSERT INTO "pipeline_runs"`, []driver.Value{sqlmock.AnyArg(), sqlmock.AnyArg(), "run-3", sqlmock.AnyArg(), "sha", "completed", "", sqlmock.AnyArg(), sqlmock.AnyArg(), updatedAt, sqlmock.AnyArg(), sqlmock.AnyArg()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, db := newWebhookMockDB(t)
			s := NewService(db, fakeRegistry{})
			tc.event.Repository, tc.event.DeliveryID, tc.event.EventType = "org/repo", "delivery-"+tc.name, "entity"
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).WithArgs("github", "org/repo", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "created_at", "updated_at"}).AddRow("11111111-1111-1111-1111-111111111111", "github", "org/repo", "SECRET", nil, nil))
			mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(tc.query).WithArgs(tc.args...).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			created, err := s.persistEvent(context.Background(), "github", "org/repo", tc.event, []byte(`{}`))
			if err != nil || !created {
				t.Fatalf("created=%v err=%v", created, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
