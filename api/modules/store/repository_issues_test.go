package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIssueUpsertIsAuthoritativeAndScoped(t *testing.T) {
	store, mock := mockStore(t)
	item := &Issue{ID: "11111111-1111-1111-1111-111111111111", RepositoryID: "22222222-2222-2222-2222-222222222222", Number: 7, Title: "", State: "closed", Status: "done", Labels: JSONB(`[]`), Assignee: ""}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "issues".*ON CONFLICT \("repository_id","number"\).*DO UPDATE SET.*"upstream_updated_at"=EXCLUDED\.upstream_updated_at.*WHERE \(issues\.upstream_updated_at IS NULL AND EXCLUDED\.upstream_updated_at IS NULL\).*EXCLUDED\.upstream_updated_at > issues\.upstream_updated_at`).WillReturnResult(sqlmockResult())
	mock.ExpectCommit()
	require.NoError(t, store.Issues.Upsert(context.Background(), item))
	mock.ExpectQuery(`SELECT \* FROM "issues" WHERE repository_id = \$1 AND number = \$2`).WithArgs(item.RepositoryID, item.Number, 1).WillReturnRows(row("id", item.ID, "repository_id", item.RepositoryID, "number", item.Number, "title", "", "state", "closed", "status", "done", "labels", []byte(`[]`), "assignee", "", "created_at", nil, "updated_at", nil))
	got, err := store.Issues.GetByNumber(context.Background(), item.RepositoryID, item.Number)
	require.NoError(t, err)
	require.Empty(t, got.Assignee)
	require.NoError(t, mock.ExpectationsWereMet())
}
