package database

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newMockDatabase(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

func expectMigrationTable(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())")).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

const appMetadataSQL = "CREATE TABLE IF NOT EXISTS app_metadata (\n    key TEXT PRIMARY KEY,\n    value TEXT NOT NULL,\n    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()\n);\n"

func expectMigrationApplied(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(appMetadataSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).
		WithArgs(1, "create_app_metadata").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func TestRunMigrationsAppliesEmbeddedSQLAndRecordsVersion(t *testing.T) {
	db, mock := newMockDatabase(t)
	expectMigrationTable(mock)
	expectMigrationApplied(mock)

	require.NoError(t, RunMigrations(context.Background(), db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunMigrationsSkipsAppliedVersion(t *testing.T) {
	db, mock := newMockDatabase(t)
	expectMigrationTable(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	require.NoError(t, RunMigrations(context.Background(), db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunMigrationsRollsBackWhenEmbeddedSQLFails(t *testing.T) {
	db, mock := newMockDatabase(t)
	expectMigrationTable(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(appMetadataSQL)).
		WillReturnError(assertionError("migration SQL rejected"))
	mock.ExpectRollback()

	err := RunMigrations(context.Background(), db)
	require.ErrorContains(t, err, "apply migration 001")
	require.NoError(t, mock.ExpectationsWereMet())
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
