package wave

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func waveDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

func waveColumns() []string {
	return []string{"id", "organization_id", "name", "brief", "state", "created_at", "updated_at"}
}

func TestRepositoryGetBindsOrganizationAndMapsMissing(t *testing.T) {
	db, mock := waveDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "waves" WHERE organization_id = $1 AND id = $2 ORDER BY "waves"."id" LIMIT $3`)).
		WithArgs("org-a", "wave-a", 1).WillReturnError(gorm.ErrRecordNotFound)
	_, err := NewRepository(db).Get(context.Background(), "org-a", "wave-a")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryListUsesTenantAndPagination(t *testing.T) {
	db, mock := waveDB(t)
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "waves" WHERE organization_id = $1`)).WithArgs("org-a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "waves" WHERE organization_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`)).WithArgs("org-a", 1, 1).WillReturnRows(sqlmock.NewRows(waveColumns()).AddRow("w2", "org-a", "two", "brief", Todo, now, now))
	rows, total, err := NewRepository(db).List(context.Background(), "org-a", 2, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryUpdateRejectsInvalidTransitionBeforeSave(t *testing.T) {
	db, mock := waveDB(t)
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "waves" WHERE organization_id = $1 AND id = $2 ORDER BY "waves"."id" LIMIT $3`)).WithArgs("org-a", "w1", 1).WillReturnRows(sqlmock.NewRows(waveColumns()).AddRow("w1", "org-a", "name", "brief", Todo, now, now))
	_, err := NewRepository(db).Update(context.Background(), "org-a", "w1", "", "", Done)
	require.ErrorIs(t, err, ErrInvalidTransition)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryDeleteBindsTenantAndReportsMissing(t *testing.T) {
	db, mock := waveDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "waves" WHERE organization_id = $1 AND id = $2`)).WithArgs("org-a", "w1").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	require.ErrorIs(t, NewRepository(db).Delete(context.Background(), "org-a", "w1"), ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
