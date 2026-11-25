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
	gitlabRepoID := "00000000-0000-0000-0000-000000000013"
	require.NoError(t, db.Exec("INSERT INTO repositories (id, organization_id, provider, full_name, webhook_secret_env_ref, gitlab_base_url) VALUES (?, ?, 'github', 'fixture/owned', 'TEST_SECRET', ''), (?, ?, 'github', 'fixture/other', 'TEST_SECRET', ''), (?, ?, 'gitlab', 'w19/pages', 'TEST_SECRET', 'https://gitlab.example.invalid/')", repoID, orgID, otherRepoID, otherOrgID, gitlabRepoID, orgID).Error)
	require.NoError(t, db.Exec("INSERT INTO agent_run_price_versions (id, organization_id, model, effective_date, input_per_million, cached_input_per_million, cache_write_per_million, output_per_million, reasoning_per_million) VALUES (?, ?, 'model', ?, 100, 100, 100, 100, 100)", uuid.NewString(), orgID, "2026-01-01").Error)
	created := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, number := range []int{1, 320} {
		require.NoError(t, db.Exec("INSERT INTO pull_requests (id, repository_id, number, title, state, head_sha, branch, created_at, updated_at) VALUES (?, ?, ?, ?, 'open', 'sha', 'main', ?, ?)", uuid.NewString(), repoID, number, "Improve hosting deployment "+strconv.Itoa(number), created, created).Error)
		require.NoError(t, db.Exec("INSERT INTO agent_runs (id, repository_id, wave, worker, model, issue_numbers, pr_number, input_tokens, cached_input_tokens, cache_write_tokens, output_tokens, reasoning_tokens, cost_usd, started_at, ended_at) VALUES (?, ?, 'W18', 'fixture', 'model', '[]', ?, 1, 0, 0, 1, 0, 12.4, ?, ?)", uuid.NewString(), repoID, number, created, created).Error)
	}
	require.NoError(t, db.Exec("INSERT INTO pull_requests (id, repository_id, number, title, state, head_sha, branch, created_at, updated_at) VALUES (?, ?, 1, 'GitLab MR', 'open', 'sha', 'main', ?, ?)", uuid.NewString(), gitlabRepoID, created, created).Error)

	repo := NewRepository(db)
	ctx := WithOrganization(context.Background(), orgID)
	for _, want := range []int64{1, 320} {
		detail, err := repo.Detail(ctx, repoID, "pr", want)
		require.NoError(t, err)
		require.Equal(t, want, detail.Item.Number)
		require.Equal(t, "https://github.com/fixture/owned/pull/"+strconv.FormatInt(want, 10), detail.Item.URL)
		require.Len(t, detail.Runs, 1)
		require.NotNil(t, detail.Runs[0].CostUSD)
		require.InDelta(t, 12.4, *detail.Runs[0].CostUSD, 1e-12)
		require.InDelta(t, 12.4, *detail.Item.CostUSD, 1e-12)
	}
	gitlabDetail, err := repo.Detail(ctx, gitlabRepoID, "pr", 1)
	require.NoError(t, err)
	require.Equal(t, "https://gitlab.example.invalid/w19/pages/-/merge_requests/1", gitlabDetail.Item.URL)
	_, err = repo.Detail(ctx, repoID, "pr", 999)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = repo.Detail(WithOrganization(context.Background(), otherOrgID), repoID, "pr", 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = repo.Detail(ctx, otherRepoID, "pr", 1)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestPostgresWorkIncludesSessionsLinkedToIssueAndPullRequest(t *testing.T) {
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
	repoID := "00000000-0000-0000-0000-000000000011"
	require.NoError(t, db.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", orgID, "owner").Error)
	require.NoError(t, db.Exec("INSERT INTO repositories (id, organization_id, provider, full_name, webhook_secret_env_ref) VALUES (?, ?, 'github', 'fixture/owned', 'TEST_SECRET')", repoID, orgID).Error)
	created := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("INSERT INTO issues (id, repository_id, number, title, state, created_at, updated_at) VALUES (?, ?, 10, 'Issue 10', 'open', ?, ?)", uuid.NewString(), repoID, created, created).Error)
	require.NoError(t, db.Exec("INSERT INTO pull_requests (id, repository_id, number, title, state, head_sha, branch, created_at, updated_at) VALUES (?, ?, 10, 'PR 10', 'open', 'sha', 'main', ?, ?)", uuid.NewString(), repoID, created, created).Error)
	for _, cost := range []any{12.4, 0.0} {
		require.NoError(t, db.Exec("INSERT INTO agent_runs (id, repository_id, wave, worker, model, issue_numbers, pr_number, input_tokens, cached_input_tokens, cache_write_tokens, output_tokens, reasoning_tokens, cost_usd, started_at, ended_at) VALUES (?, ?, 'W18', 'fixture', 'model', '[10]', 10, 1, 0, 0, 1, 0, ?, ?, ?)", uuid.NewString(), repoID, cost, created, created).Error)
	}
	repo := NewRepository(db)
	ctx := WithOrganization(context.Background(), orgID)
	items, total, err := repo.List(ctx, repoID, "", "", 1, 50)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	for _, item := range items {
		require.EqualValues(t, 2, item.RunsCount)
		require.NotNil(t, item.CostUSD)
		require.InDelta(t, 12.4, *item.CostUSD, 1e-12)
	}
	for _, kind := range []string{"issue", "pr"} {
		detail, err := repo.Detail(ctx, repoID, kind, 10)
		require.NoError(t, err)
		require.EqualValues(t, 2, detail.Item.RunsCount)
		require.Len(t, detail.Runs, 2)
		require.NotNil(t, detail.Item.CostUSD)
		require.InDelta(t, 12.4, *detail.Item.CostUSD, 1e-12)
		require.NotNil(t, detail.Runs[0].CostUSD)
		require.InDelta(t, 12.4, *detail.Runs[0].CostUSD, 1e-12)
		require.NotNil(t, detail.Runs[1].CostUSD)
		require.Zero(t, *detail.Runs[1].CostUSD)
	}
}
