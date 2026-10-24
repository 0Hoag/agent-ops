package store

import (
	"context"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
)

func TestPipelineUpsertClearsFinishedSnapshotAndLooksUpSHA(t *testing.T) {
	store, mock := mockStore(t)
	item := &PipelineRun{ID: "11111111-1111-1111-1111-111111111111", RepositoryID: "22222222-2222-2222-2222-222222222222", ExternalID: "run-1", SHA: "abc", Status: "queued", Conclusion: ""}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "pipeline_runs".*ON CONFLICT \("repository_id","external_id"\).*DO UPDATE SET.*"upstream_updated_at"=EXCLUDED\.upstream_updated_at.*WHERE \(pipeline_runs\.upstream_updated_at IS NULL AND EXCLUDED\.upstream_updated_at IS NULL\).*EXCLUDED\.upstream_updated_at > pipeline_runs\.upstream_updated_at`).WillReturnResult(sqlmockResult())
	mock.ExpectCommit()
	require.NoError(t, store.PipelineRuns.Upsert(context.Background(), item))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "pipeline_runs" WHERE repository_id = $1 AND sha = $2 ORDER BY started_at DESC NULLS LAST`)).WithArgs(item.RepositoryID, item.SHA).WillReturnRows(row("id", item.ID, "repository_id", item.RepositoryID, "external_id", item.ExternalID, "pr_number", nil, "sha", item.SHA, "status", item.Status, "conclusion", "", "started_at", nil, "finished_at", nil, "created_at", nil, "updated_at", nil))
	items, err := store.PipelineRuns.ListBySHA(context.Background(), item.RepositoryID, item.SHA)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Empty(t, items[0].Conclusion)
	require.NoError(t, mock.ExpectationsWereMet())
}
