package board

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresBoardAcceptance(t *testing.T) {
	dsn := os.Getenv("W2A_BOARD_ACCEPTANCE_DSN")
	if dsn == "" {
		t.Skip("set W2A_BOARD_ACCEPTANCE_DSN to run disposable PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.RunMigrations(context.Background(), db))
	ctx := context.Background()
	ids := []store.UUID{"a1000000-0000-0000-0000-000000000001", "a2000000-0000-0000-0000-000000000002"}
	require.NoError(t, db.Where("repository_id IN ?", ids).Delete(&store.Issue{}).Error)
	require.NoError(t, db.Where("repository_id IN ?", ids).Delete(&store.PullRequest{}).Error)
	require.NoError(t, db.Where("repository_id IN ?", ids).Delete(&store.PipelineRun{}).Error)
	for i, id := range ids {
		require.NoError(t, db.Exec("INSERT INTO repositories (id, provider, full_name, webhook_secret_env_ref) VALUES (?, 'github', ?, ?) ON CONFLICT (id) DO NOTHING", id, []string{"fixture/alpha", "fixture/beta"}[i], "MUST_NOT_LEAK").Error)
	}
	t.Cleanup(func() {
		_ = db.Where("repository_id IN ?", ids).Delete(&store.Issue{}).Error
		_ = db.Where("repository_id IN ?", ids).Delete(&store.PullRequest{}).Error
		_ = db.Where("repository_id IN ?", ids).Delete(&store.PipelineRun{}).Error
		_ = db.Where("id IN ?", ids).Delete(&store.Repository{}).Error
	})
	statuses := []string{"todo", "in_progress", "review", "blocked", "done"}
	for i, status := range statuses {
		labels, _ := json.Marshal([]string{"fixture", status})
		issue := store.Issue{ID: store.NewUUID(), RepositoryID: ids[0], Number: int64(i + 1), Title: "alpha " + status, State: "open", Status: status, Labels: store.JSONB(labels), Assignee: "worker"}
		require.NoError(t, db.Create(&issue).Error)
	}
	other := store.Issue{ID: store.NewUUID(), RepositoryID: ids[1], Number: 1, Title: "beta same issue", State: "open", Status: "review", Labels: store.JSONB(`[]`)}
	require.NoError(t, db.Create(&other).Error)
	for i, id := range ids {
		closedIssue := 1
		if i == 0 {
			closedIssue = 5
		}
		closes, _ := json.Marshal([]int{closedIssue})
		title := "PR " + string(id[:4])
		if i == 1 {
			title = "beta PR"
		}
		pr := store.PullRequest{ID: store.NewUUID(), RepositoryID: id, Number: int64(10 + i), Title: title, State: "open", HeadSHA: "shared-sha", Branch: "feature", Closes: store.JSONB(closes)}
		require.NoError(t, db.Create(&pr).Error)
		externalID := "run-" + string(id[:4])
		run := store.PipelineRun{ID: store.NewUUID(), RepositoryID: id, ExternalID: externalID, SHA: "shared-sha", Status: "completed", Conclusion: "success"}
		require.NoError(t, db.Create(&run).Error)
	}
	repoRows, repoMeta, err := NewRepository(db).ListRepositories(ctx, 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, repoMeta.Total)
	require.Len(t, repoRows, 1)
	encoded, err := json.Marshal(repoRows)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "MUST_NOT_LEAK")
	require.NotContains(t, string(encoded), "webhook_secret")
	items, meta, err := NewRepository(db).ListIssues(ctx, string(ids[0]), 1, 2)
	require.NoError(t, err)
	require.EqualValues(t, 5, meta.Total)
	require.Len(t, items, 2)
	require.Equal(t, "done", items[0].Status)
	require.Len(t, items[0].PullRequests, 1)
	require.Len(t, items[0].PullRequests[0].PipelineRuns, 1)
	require.Equal(t, "alpha blocked", items[1].Title)
	for _, item := range items {
		require.NotEmpty(t, item.Labels)
	}
	empty, _, err := NewRepository(db).ListIssues(ctx, string(ids[1]), 2, 100)
	require.NoError(t, err)
	require.Empty(t, empty)
	alpha, _, err := NewRepository(db).ListIssues(ctx, string(ids[0]), 1, 100)
	require.NoError(t, err)
	beta, _, err := NewRepository(db).ListIssues(ctx, string(ids[1]), 1, 100)
	require.NoError(t, err)
	require.Len(t, alpha, 5)
	require.Len(t, beta, 1)
	gotStatuses := make([]string, 0, len(alpha))
	for _, item := range alpha {
		gotStatuses = append(gotStatuses, item.Status)
	}
	require.ElementsMatch(t, statuses, gotStatuses)
	require.Equal(t, "beta same issue", beta[0].Title)
	require.Len(t, alpha[0].PullRequests, 1)
	require.EqualValues(t, 10, alpha[0].PullRequests[0].Number)
	require.Equal(t, "PR "+string(ids[0][:4]), alpha[0].PullRequests[0].Title)
	require.Equal(t, "run-a100", alpha[0].PullRequests[0].PipelineRuns[0].ExternalID)
	require.EqualValues(t, 11, beta[0].PullRequests[0].Number)
	require.Equal(t, "run-a200", beta[0].PullRequests[0].PipelineRuns[0].ExternalID)
	for _, item := range alpha[1:] {
		require.Empty(t, item.PullRequests)
	}
}
