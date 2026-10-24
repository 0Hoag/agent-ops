package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresAgentRunRepository(t *testing.T) {
	dsn := os.Getenv("W3A_AGENT_RUN_ACCEPTANCE_DSN")
	if dsn == "" {
		t.Skip("set W3A_AGENT_RUN_ACCEPTANCE_DSN to use disposable PostgreSQL")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.RunMigrations(context.Background(), db))
	ctx := context.Background()
	ids := []store.UUID{"a3000000-0000-0000-0000-000000000003", "a4000000-0000-0000-0000-000000000004"}
	for i, id := range ids {
		require.NoError(t, db.Exec("INSERT INTO repositories (id, provider, full_name, webhook_secret_env_ref) VALUES (?, 'github', ?, 'FIXTURE') ON CONFLICT (id) DO NOTHING", id, []string{"fixture/run-a", "fixture/run-b"}[i]).Error)
	}
	t.Cleanup(func() {
		_ = db.Where("repository_id IN ?", ids).Delete(&AgentRun{}).Error
		_ = db.Where("id IN ?", ids).Delete(&store.Repository{}).Error
	})
	repo := NewRepository(db)
	runs := []AgentRun{
		{ID: store.NewUUID(), RepositoryID: ids[0], Wave: "w3a", Worker: "alpha", Model: "known", IssueNumbers: store.JSONB(`[24,25]`), InputTokens: 10},
		{ID: store.NewUUID(), RepositoryID: ids[0], Wave: "w3b", Worker: "beta", Model: "known", IssueNumbers: store.JSONB(`[25]`), InputTokens: 20},
		{ID: store.NewUUID(), RepositoryID: ids[1], Wave: "w3a", Worker: "alpha", Model: "known", IssueNumbers: store.JSONB(`[24]`), InputTokens: 30},
	}
	for i := range runs {
		run := &runs[i]
		run.StartedAt = time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC)
		run.EndedAt = run.StartedAt
		require.NoError(t, repo.Create(ctx, run))
	}
	all, err := repo.List(ctx, Filter{RepositoryID: string(ids[0])})
	require.NoError(t, err)
	require.Len(t, all, 2)
	byWave, err := repo.List(ctx, Filter{RepositoryID: string(ids[0]), Wave: "w3a"})
	require.NoError(t, err)
	require.Len(t, byWave, 1)
	require.Equal(t, int64(10), byWave[0].InputTokens)
	byIssue, err := repo.List(ctx, Filter{RepositoryID: string(ids[0]), IssueNumber: 24})
	require.NoError(t, err)
	require.Len(t, byIssue, 1)
	require.Equal(t, "w3a", byIssue[0].Wave)
	groups, err := repo.Aggregate(ctx, Filter{RepositoryID: string(ids[0])}, map[string]Price{"known": {InputPerMillion: 1}})
	require.NoError(t, err)
	require.Len(t, groups, 3)
	for _, group := range groups {
		if group.Wave == "w3a" && group.Worker == "alpha" && *group.IssueNumber == 24 {
			require.NotNil(t, group.IssueNumber)
			require.EqualValues(t, 24, *group.IssueNumber)
			require.EqualValues(t, 10, group.InputTokens)
			require.NotNil(t, group.CostUSD)
		}
	}
	encoded, err := json.Marshal(groups)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "w3a")
}
