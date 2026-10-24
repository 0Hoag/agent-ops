package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresStaleSnapshotPredicateControl(t *testing.T) {
	dsn := os.Getenv("W1D_ACCEPTANCE_DSN")
	if dsn == "" {
		t.Skip("set W1D_ACCEPTANCE_DSN to run disposable PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.RunMigrations(context.Background(), db))
	repositoryID := UUID("22222222-2222-2222-2222-222222222222")
	require.NoError(t, db.Exec("INSERT INTO repositories (id, provider, full_name, webhook_secret_env_ref) VALUES (?, 'github', ?, 'TEST_SECRET') ON CONFLICT (id) DO NOTHING", repositoryID, "w1d-control").Error)
	require.NoError(t, db.Exec("DELETE FROM issues WHERE repository_id = ?", repositoryID).Error)
	newer, older := time.Date(2026, 10, 24, 1, 0, 0, 0, time.UTC), time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{newer, older} {
		state, title := "open", "old snapshot"
		if at.Equal(newer) {
			state, title = "closed", "new snapshot"
		}
		i := &Issue{ID: UUID("33333333-3333-3333-3333-333333333333"), RepositoryID: repositoryID, Number: 1, Title: title, State: state, Status: state, Labels: JSONB(`[]`), UpstreamUpdatedAt: &at}
		require.NoError(t, db.Exec(`INSERT INTO issues (id, repository_id, number, title, state, status, labels, upstream_updated_at) VALUES (?, ?, 1, ?, ?, ?, '[]'::jsonb, ?) ON CONFLICT (repository_id, number) DO UPDATE SET title = EXCLUDED.title, state = EXCLUDED.state, status = EXCLUDED.status, labels = EXCLUDED.labels, assignee = EXCLUDED.assignee, upstream_updated_at = EXCLUDED.upstream_updated_at, updated_at = NOW()`, i.ID, i.RepositoryID, i.Title, i.State, i.Status, i.UpstreamUpdatedAt).Error)
	}
	var got Issue
	require.NoError(t, db.Where("repository_id = ? AND number = 1", repositoryID).First(&got).Error)
	require.Equal(t, "closed", got.State, "predicate removal should expose stale overwrite: expected newer state closed, got %q", got.State)
	require.Equal(t, "new snapshot", got.Title, "predicate removal should expose stale overwrite: expected newer value, got %q", got.Title)
	require.True(t, newer.Equal(*got.UpstreamUpdatedAt))
}
