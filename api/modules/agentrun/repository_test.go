package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/DATA-DOG/go-sqlmock"
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

func TestAggregateUnknownCostIsOrderIndependent(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	repo := NewRepository(db)
	knownPrice := map[string]Price{"known": {InputPerMillion: 1}}
	knownPrice["zero"] = Price{}
	for _, tc := range []struct {
		name   string
		models []string
		known  bool
		cost   float64
	}{
		{name: "known unknown known", models: []string{"known", "unknown", "known"}},
		{name: "unknown known", models: []string{"unknown", "known"}},
		{name: "known only", models: []string{"known", "known"}, known: true, cost: 2e-6},
		{name: "known zero", models: []string{"zero"}, known: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := sqlmock.NewRows([]string{"id", "repository_id", "wave", "worker", "model", "issue_numbers", "input_tokens", "cached_input_tokens", "cache_write_tokens", "output_tokens", "reasoning_tokens", "cost_usd", "started_at", "ended_at", "created_at", "updated_at"})
			for i, model := range tc.models {
				tokens := int64(1)
				if tc.name == "known zero" {
					tokens = 0
				}
				rows.AddRow(store.NewUUID(), nil, "w", "worker", model, []byte(`[1]`), tokens, 0, 0, 0, 0, nil, time.Date(2026, 1, len(tc.models)-i, 0, 0, 0, 0, time.UTC), nil, nil, nil)
			}
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_runs" ORDER BY started_at DESC`)).WillReturnRows(rows)
			groups, err := repo.Aggregate(context.Background(), Filter{}, knownPrice)
			require.NoError(t, err)
			require.Len(t, groups, 1)
			if tc.known {
				require.NotNil(t, groups[0].CostUSD)
				require.InDelta(t, tc.cost, *groups[0].CostUSD, 1e-12)
			} else {
				require.Nil(t, groups[0].CostUSD)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
