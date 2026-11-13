package work

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func mockWorkRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true,
	})
	require.NoError(t, err)
	return NewRepository(db), mock
}

func TestListScopesRepositoryAndOrganizationAndAppliesFilters(t *testing.T) {
	repo, mock := mockWorkRepository(t)
	ctx := WithOrganization(context.Background(), "org-a")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "repositories" WHERE id = $1 AND organization_id = $2`)).
		WithArgs("repo-a", "org-a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(WITH work AS`).
		WithArgs("repo-a", "repo-a", "repo-a", "org-a", "org-a", "open", "%search%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`WITH work AS[\s\S]*LIMIT 10 OFFSET 10`).
		WithArgs("repo-a", "repo-a", "repo-a", "org-a", "org-a", "open", "%search%").
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "number", "title", "url", "author", "state", "ci_status", "created_at", "updated_at",
			"runs_count", "cost_usd", "tokens", "last_activity",
		}).AddRow("pr", 42, "Search result", "", "", "open", "success", created, created, 2, 0.5, 100, created))

	items, total, err := repo.List(ctx, "repo-a", "open", "search", 2, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, []Item{{Kind: "pr", Number: 42, Title: "Search result", State: "open", CIStatus: "success", CreatedAt: created, UpdatedAt: created, RunsCount: 2, CostUSD: floatPtr(0.5), Tokens: 100, LastActivity: created}}, items)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListReturnsNotFoundWithoutQueryingWorkForOtherOrganization(t *testing.T) {
	repo, mock := mockWorkRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "repositories" WHERE id = $1 AND organization_id = $2`)).
		WithArgs("repo-a", "org-b").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	items, total, err := repo.List(WithOrganization(context.Background(), "org-b"), "repo-a", "", "", 1, 50)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Nil(t, items)
	require.Zero(t, total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDetailScopesRequestedIdentityAndTenant(t *testing.T) {
	repo, mock := mockWorkRepository(t)
	ctx := WithOrganization(context.Background(), "org-a")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "repositories" WHERE id = $1 AND organization_id = $2`)).
		WithArgs("repo-a", "org-a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`WITH work AS[\s\S]*WHERE EXISTS \(SELECT 1 FROM repositories WHERE id=x\.repository_id AND organization_id=\$5\)[\s\S]*AND x\.kind=\$6[\s\S]*AND x\.number = \$7`).
		WithArgs("repo-a", "repo-a", "repo-a", "org-a", "org-a", "pr", int64(320)).
		WillReturnRows(sqlmock.NewRows([]string{
			"kind", "number", "title", "url", "author", "state", "ci_status", "created_at", "updated_at",
			"runs_count", "cost_usd", "tokens", "last_activity",
		}).AddRow("pr", 320, "Requested pull request", "", "", "open", "", time.Now(), time.Now(), 1, nil, 1, time.Now()))
	mock.ExpectQuery(`SELECT agent_runs\.[\s\S]*FROM "agent_runs"[\s\S]*WHERE agent_runs\.repository_id=\$1 AND pr_number=\$2`).
		WithArgs("repo-a", int64(320)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "repository_id", "wave", "worker", "model", "issue_numbers", "pr_number", "input_tokens",
			"cached_input_tokens", "cache_write_tokens", "output_tokens", "reasoning_tokens", "started_at", "ended_at", "calculated_cost",
		}))

	detail, err := repo.Detail(ctx, "repo-a", "pr", 320)
	require.NoError(t, err)
	require.EqualValues(t, 320, detail.Item.Number)
	require.Empty(t, detail.Runs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func floatPtr(v float64) *float64 { return &v }
