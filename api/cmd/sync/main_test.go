package main

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPersistBackfillTwiceUpsertsStableEntitiesAndNormalizedStates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)

	st := store.New(orm)
	repoID := store.NewUUID()
	mergedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	result := scm.BackfillResult{
		Issues:       []scm.Issue{{Number: 31, Title: "closed issue", State: "closed", Status: scm.StatusDone, Labels: []string{"accepted"}, Assignee: "reviewer"}},
		PullRequests: []scm.PullRequest{{Number: 32, Title: "merged PR", State: "closed", Merged: true, MergedAt: &mergedAt, HeadSHA: "abc", HeadBranch: "main"}},
	}
	issueInsert := regexp.MustCompile(`INSERT INTO "issues".*ON CONFLICT.*WHERE .*upstream_updated_at`)
	pullInsert := regexp.MustCompile(`INSERT INTO "pull_requests".*ON CONFLICT.*WHERE .*upstream_updated_at`)
	pipelineInsert := regexp.MustCompile(`INSERT INTO "pipeline_runs".*ON CONFLICT.*WHERE .*upstream_updated_at`)
	startedAt := time.Date(2026, 1, 2, 2, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC)
	pipelinePRNumber := 32
	result.PipelineRuns = []scm.PipelineRun{{ExternalID: "run-33", PRNumber: &pipelinePRNumber, SHA: "def", Status: "completed", Conclusion: "success", StartedAt: startedAt, FinishedAt: &finishedAt}}
	for pass := 0; pass < 2; pass++ {
		mock.ExpectBegin()
		mock.ExpectExec(issueInsert.String()).WithArgs(sqlmock.AnyArg(), repoID, int64(31), "closed issue", "closed", "done", []byte(`["accepted"]`), "reviewer", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		mock.ExpectBegin()
		mock.ExpectExec(pullInsert.String()).WithArgs(sqlmock.AnyArg(), repoID, int64(32), "merged PR", "closed", "abc", "main", []byte("null"), mergedAt, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		mock.ExpectBegin()
		mock.ExpectExec(pipelineInsert.String()).WithArgs(sqlmock.AnyArg(), repoID, "run-33", int64(32), "def", "completed", "success", startedAt, finishedAt, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		require.NoError(t, persistBackfill(context.Background(), st, repoID, result))
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
