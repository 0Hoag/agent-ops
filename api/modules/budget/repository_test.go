package budget

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func mockBudgetRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return NewRepository(db), mock
}

func budgetRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "organization_id", "repository_id", "wave", "token_limit", "usd_limit", "starts_at", "ends_at", "created_at", "updated_at"})
}

func TestRepositoryCRUDIsOrganizationScoped(t *testing.T) {
	r, mock := mockBudgetRepository(t)
	ctx := context.Background()
	org := "org-a"
	id := "budget-a"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	token := int64(100)
	b := &Budget{ID: store.UUID(id), OrganizationID: store.UUID(org), Wave: "W1", TokenLimit: &token, StartsAt: start, EndsAt: end}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "budgets" WHERE organization_id = $1 AND id = $2 ORDER BY "budgets"."id" LIMIT $3`)).
		WithArgs(org, id, 1).WillReturnRows(budgetRows().AddRow(id, org, nil, "W1", token, nil, start, end, start, start))
	got, err := r.Get(ctx, org, id)
	require.NoError(t, err)
	require.Equal(t, org, string(got.OrganizationID))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "budgets" SET .* WHERE organization_id = \$[0-9]+ AND id = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, r.Update(ctx, org, id, b))

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "budgets" WHERE organization_id = $1 AND id = $2`)).
		WithArgs(org, id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, r.Delete(ctx, org, id))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryBelongsChecksBothRepositoryAndOrganization(t *testing.T) {
	r, mock := mockBudgetRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "repositories" WHERE id = $1 AND organization_id = $2`)).
		WithArgs("repo-a", "org-a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	ok, err := r.RepositoryBelongs(context.Background(), "org-a", "repo-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}
