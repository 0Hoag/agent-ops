package insight

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func ev(id, status string, at time.Time, issue int64) event {
	return event{WebhookEvent: store.WebhookEvent{DeliveryID: id, EventType: "issues", ReceivedAt: at}, status: status, issue: &issue}
}

func TestSendBackDeduplicatedReturn(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := sendBack([]event{ev("a", "in_progress", t0, 7), ev("b", "review", t0.Add(time.Hour), 7), ev("b", "review", t0.Add(time.Hour), 7), ev("c", "in_progress", t0.Add(2*time.Hour), 7), ev("e", "in_progress", t0, 8), ev("f", "review", t0.Add(time.Hour), 8)})
	require.Equal(t, 2, got.Denominator)
	require.Equal(t, 1, got.Numerator)
	require.Equal(t, .5, got.Rate)
}

func TestFirstCIUsesFirstConclusionPerSHA(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	p := int64(1)
	p2 := int64(2)
	got := firstCI([]event{{WebhookEvent: store.WebhookEvent{DeliveryID: "1", ReceivedAt: base}, pr: &p, pipelineID: "1", startedAt: base, sha: "a", conclusion: "success", sourceAt: base}, {WebhookEvent: store.WebhookEvent{DeliveryID: "2", ReceivedAt: base.Add(time.Minute)}, pr: &p, pipelineID: "2", startedAt: base.Add(time.Minute), sha: "a", conclusion: "failure", sourceAt: base.Add(time.Minute)}, {WebhookEvent: store.WebhookEvent{DeliveryID: "3", ReceivedAt: base}, pr: &p2, pipelineID: "3", startedAt: base, sha: "b", conclusion: "failure", sourceAt: base}, {WebhookEvent: store.WebhookEvent{DeliveryID: "4", ReceivedAt: base}, pr: &p, sha: "c"}})
	require.Equal(t, 2, got.Eligible)
	require.Equal(t, 1, got.Successful)
	require.Equal(t, .5, got.Rate)
}

func TestBuildDecodesRetainedNestedProviderPayloads(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = sqlDB.Close() }()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	repoID := store.UUID("d0000000-0000-4000-8000-000000000001")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []store.WebhookEvent{
		{RepositoryID: repoID, Provider: "github", EventType: "issues", DeliveryID: "gh-1", ReceivedAt: base, Payload: store.JSONB(`{"issue":{"number":7,"state":"open","labels":[{"name":"status:in progress"}]}}`)},
		{RepositoryID: repoID, Provider: "github", EventType: "issues", DeliveryID: "gh-2", ReceivedAt: base.Add(time.Hour), Payload: store.JSONB(`{"issue":{"number":7,"state":"open","labels":[{"name":"status:review"}]}}`)},
		{RepositoryID: repoID, Provider: "github", EventType: "issues", DeliveryID: "gh-3", ReceivedAt: base.Add(2 * time.Hour), Payload: store.JSONB(`{"issue":{"number":7,"state":"open","labels":[{"name":"status:in progress"}]}}`)},
		{RepositoryID: repoID, Provider: "github", EventType: "issues", DeliveryID: "gh-4", ReceivedAt: base.Add(3 * time.Hour), Payload: store.JSONB(`{"issue":{"number":7,"state":"closed","labels":[]}}`)},
		{RepositoryID: repoID, Provider: "gitlab", EventType: "Issue Hook", DeliveryID: "gl-1", ReceivedAt: base, Payload: store.JSONB(`{"object_attributes":{"iid":12,"state":"opened","labels":["Status::Todo"]}}`)},
		{RepositoryID: repoID, Provider: "gitlab", EventType: "Issue Hook", DeliveryID: "gl-2", ReceivedAt: base.Add(time.Hour), Payload: store.JSONB(`{"object_attributes":{"iid":12,"state":"opened","labels":["Status::Review"]}}`)},
		{RepositoryID: repoID, Provider: "gitlab", EventType: "Issue Hook", DeliveryID: "gl-3", ReceivedAt: base.Add(2 * time.Hour), Payload: store.JSONB(`{"object_attributes":{"iid":12,"state":"opened","labels":["Status::In Progress"]}}`)},
		{RepositoryID: repoID, Provider: "gitlab", EventType: "Issue Hook", DeliveryID: "gl-4", ReceivedAt: base.Add(3 * time.Hour), Payload: store.JSONB(`{"object_attributes":{"iid":13,"state":"opened","labels":["Status::Blocked"]}}`)},
		{RepositoryID: repoID, Provider: "gitlab", EventType: "Issue Hook", DeliveryID: "gl-5", ReceivedAt: base.Add(4 * time.Hour), Payload: store.JSONB(`{"object_attributes":{"iid":12,"state":"closed","labels":[]}}`)},
		{RepositoryID: repoID, Provider: "gitlab", EventType: "Merge Request Hook", DeliveryID: "mr-1", ReceivedAt: base.Add(5 * time.Hour), Payload: store.JSONB(`{"object_attributes":{"iid":12,"state":"opened","labels":["Status::Review"]}}`)},
		{RepositoryID: repoID, Provider: "github", EventType: "workflow_run", DeliveryID: "ci-1", ReceivedAt: base.Add(time.Minute), Payload: store.JSONB(`{"workflow_run":{"id":1,"head_sha":"abc","conclusion":"success","run_started_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:02:00Z","pull_requests":[{"number":8}]}}`)},
		{RepositoryID: repoID, Provider: "github", EventType: "workflow_run", DeliveryID: "ci-no-sha", ReceivedAt: base.Add(2 * time.Minute), Payload: store.JSONB(`{"workflow_run":{"id":2,"conclusion":"success","run_started_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:03:00Z","pull_requests":[{"number":9}]}}`)},
		{RepositoryID: repoID, Provider: "github", EventType: "workflow_run", DeliveryID: "ci-no-id", ReceivedAt: base.Add(3 * time.Minute), Payload: store.JSONB(`{"workflow_run":{"head_sha":"def","conclusion":"success","run_started_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:04:00Z","pull_requests":[{"number":10}]}}`)},
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "repositories" WHERE id = $1`)).WithArgs(string(repoID)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	result := sqlmock.NewRows([]string{"id", "repository_id", "provider", "event_type", "delivery_id", "payload", "received_at", "created_at", "updated_at"})
	for i, row := range rows {
		result.AddRow(fmt.Sprintf("event-%d", i), string(row.RepositoryID), row.Provider, row.EventType, row.DeliveryID, []byte(row.Payload), row.ReceivedAt, row.ReceivedAt, row.ReceivedAt)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "webhook_events" WHERE repository_id = $1 ORDER BY received_at ASC`)).WithArgs(string(repoID)).WillReturnRows(result)
	got, err := NewRepository(db).Build(context.Background(), string(repoID))
	require.NoError(t, err)
	require.False(t, got.History.Complete)
	require.Equal(t, 2, got.SendBack.Numerator)
	require.Equal(t, 1, got.SendBack.Denominator)
	require.Equal(t, 2.0, got.SendBack.Rate)
	require.Equal(t, 1, got.FirstCI.Eligible)
	require.Equal(t, 1, got.FirstCI.Successful)
	require.Equal(t, 2, got.CycleTime.Completed)
	require.Equal(t, "ci-1", got.FirstCI.Evidence[0].DeliveryID)
	require.Equal(t, []string{"gh-3", "gl-3"}, evidenceDeliveryIDs(got.SendBack.Evidence))
	require.NoError(t, mock.ExpectationsWereMet())
}

func evidenceDeliveryIDs(evidence []Evidence) []string {
	ids := make([]string, len(evidence))
	for i, item := range evidence {
		ids[i] = item.DeliveryID
	}
	return ids
}

func TestDecodeRealProviderFixtures(t *testing.T) {
	for _, tc := range []struct{ provider, event, file string }{{"github", "issues", "../../pkg/handlers/scm/github/testdata/issues.json"}, {"github", "workflow_run", "../../pkg/handlers/scm/github/testdata/workflow_run.json"}, {"gitlab", "issue hook", "testdata/gitlab_issue.json"}} {
		payload, err := os.ReadFile(filepath.Clean(tc.file))
		require.NoError(t, err)
		got := decode(store.WebhookEvent{Provider: tc.provider, EventType: tc.event, DeliveryID: tc.file, ReceivedAt: time.Now(), Payload: store.JSONB(payload)})
		if tc.event == "issues" {
			require.NotNil(t, got.issue)
			require.Equal(t, int64(7), *got.issue)
			require.Equal(t, "blocked", got.status)
		} else if tc.event == "workflow_run" {
			require.Equal(t, "abc", got.sha)
			require.Equal(t, "success", got.conclusion)
			require.NotNil(t, got.pr)
			require.Equal(t, int64(8), *got.pr)
		} else if tc.provider == "gitlab" {
			require.NotNil(t, got.issue)
			require.Equal(t, int64(12), *got.issue)
			require.Equal(t, "in_progress", got.status)
		}
	}
}

func TestCycleTimeExcludesIncompleteAndIncludesBlocked(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	got := cycle([]event{ev("a", "in_progress", base, 1), ev("b", "blocked", base.Add(time.Hour), 1), ev("c", "done", base.Add(3*time.Hour), 1), ev("d", "in_progress", base, 2)})
	require.Equal(t, 1, got.Completed)
	require.Equal(t, float64(3*time.Hour/time.Second), got.MedianSeconds)
}

func TestCycleTimeReopenStartsAnotherInterval(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	got := cycle([]event{ev("a", "in_progress", base, 9), ev("b", "done", base.Add(time.Hour), 9), ev("c", "in_progress", base.Add(2*time.Hour), 9), ev("d", "done", base.Add(4*time.Hour), 9)})
	require.Equal(t, 2, got.Completed)
	require.Equal(t, float64(90*time.Minute/time.Second), got.MedianSeconds)
}

func TestSuggestionsRequireRecurringFailures(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	got := suggestions([]event{{WebhookEvent: store.WebhookEvent{DeliveryID: "1", ReceivedAt: base}, sha: "same", conclusion: "failure"}, {WebhookEvent: store.WebhookEvent{DeliveryID: "2", ReceivedAt: base.Add(time.Minute)}, sha: "same", conclusion: "failure"}, {WebhookEvent: store.WebhookEvent{DeliveryID: "3", ReceivedAt: base}, sha: "one", conclusion: "failure"}})
	require.Len(t, got, 1)
	require.Equal(t, 2, got[0].Occurrences)
	require.Len(t, got[0].Evidence, 2)
}
