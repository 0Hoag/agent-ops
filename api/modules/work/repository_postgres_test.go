package work

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// WORK_TEST_DATABASE_URL must name an owned disposable database.
func TestPostgresDetailKeepsRequestedIdentityAndTenant(t *testing.T) {
	dsn := os.Getenv("WORK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WORK_TEST_DATABASE_URL for real PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	schema := "work_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	require.NoError(t, db.Exec("SET search_path TO "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, database.RunMigrations(context.Background(), db))

	orgID := "00000000-0000-0000-0000-000000000001"
	otherOrgID := "00000000-0000-0000-0000-000000000002"
	repoID := "00000000-0000-0000-0000-000000000011"
	otherRepoID := "00000000-0000-0000-0000-000000000012"
	require.NoError(t, db.Exec("INSERT INTO organizations (id, name) VALUES (?, ?), (?, ?)", orgID, "owner", otherOrgID, "other").Error)
	require.NoError(t, db.Exec("INSERT INTO repositories (id, organization_id, provider, full_name, webhook_secret_env_ref) VALUES (?, ?, 'github', 'fixture/owned', 'TEST_SECRET'), (?, ?, 'github', 'fixture/other', 'TEST_SECRET')", repoID, orgID, otherRepoID, otherOrgID).Error)
	created := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, number := range []int{1, 320} {
		require.NoError(t, db.Exec("INSERT INTO pull_requests (id, repository_id, number, title, state, head_sha, branch, created_at, updated_at) VALUES (?, ?, ?, ?, 'open', 'sha', 'main', ?, ?)", uuid.NewString(), repoID, number, "Improve hosting deployment "+strconv.Itoa(number), created, created).Error)
		require.NoError(t, db.Exec("INSERT INTO agent_runs (id, repository_id, wave, worker, model, issue_numbers, pr_number, input_tokens, cached_input_tokens, cache_write_tokens, output_tokens, reasoning_tokens, started_at, ended_at) VALUES (?, ?, 'W18', 'fixture', 'model', '[]', ?, 1, 0, 0, 1, 0, ?, ?)", uuid.NewString(), repoID, number, created, created).Error)
	}

	repo := NewRepository(db)
	ctx := WithOrganization(context.Background(), orgID)
	for _, want := range []int64{1, 320} {
		detail, err := repo.Detail(ctx, repoID, "pr", want)
		require.NoError(t, err)
		require.Equal(t, want, detail.Item.Number)
		require.Len(t, detail.Runs, 1)
	}
	_, err = repo.Detail(ctx, repoID, "pr", 999)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = repo.Detail(WithOrganization(context.Background(), otherOrgID), repoID, "pr", 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = repo.Detail(ctx, otherRepoID, "pr", 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
