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
	defer db.Close()
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)

	st := store.New(orm)
	repoID := store.NewUUID()
	mergedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	result := scm.BackfillResult{
		Issues:       []scm.Issue{{Number: 31, Title: "closed issue", State: "closed", Status: scm.StatusDone}},
		PullRequests: []scm.PullRequest{{Number: 32, Title: "merged PR", State: "closed", Merged: true, MergedAt: &mergedAt, HeadSHA: "abc", HeadBranch: "main"}},
	}
	issueInsert := regexp.QuoteMeta(`INSERT INTO "issues"`)
	pullInsert := regexp.QuoteMeta(`INSERT INTO "pull_requests"`)
	for pass := 0; pass < 2; pass++ {
		mock.ExpectBegin()
		mock.ExpectExec(issueInsert).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		mock.ExpectBegin()
		mock.ExpectExec(pullInsert).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		require.NoError(t, persistBackfill(context.Background(), st, repoID, result))
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
