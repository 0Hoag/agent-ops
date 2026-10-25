package store

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPullRequestUpsertClearsClosesAndLooksUpLinkedIssue(t *testing.T) {
	store, mock := mockStore(t)
	item := &PullRequest{ID: "11111111-1111-1111-1111-111111111111", RepositoryID: "22222222-2222-2222-2222-222222222222", Number: 9, Title: "updated", State: "open", HeadSHA: "new", Branch: "main", Closes: JSONB(`[]`)}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "pull_requests".*ON CONFLICT \("repository_id","number"\).*DO UPDATE SET.*"upstream_updated_at"=EXCLUDED\.upstream_updated_at.*WHERE \(pull_requests\.upstream_updated_at IS NULL AND EXCLUDED\.upstream_updated_at IS NULL\).*EXCLUDED\.upstream_updated_at > pull_requests\.upstream_updated_at`).WillReturnResult(sqlmockResult())
	mock.ExpectCommit()
	require.NoError(t, store.PullRequests.Upsert(context.Background(), item))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "pull_requests" WHERE repository_id = $1 AND closes @> $2::jsonb ORDER BY number DESC`)).WithArgs(item.RepositoryID, `[9]`).WillReturnRows(row("id", item.ID, "repository_id", item.RepositoryID, "number", item.Number, "title", item.Title, "state", item.State, "head_sha", item.HeadSHA, "branch", item.Branch, "closes", []byte(`[]`), "merged_at", nil, "created_at", nil, "updated_at", nil))
	items, err := store.PullRequests.ListByClosedIssue(context.Background(), item.RepositoryID, 9)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}
