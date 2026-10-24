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

const storeSQL = "CREATE TABLE IF NOT EXISTS repositories (\n    id UUID PRIMARY KEY,\n    provider TEXT NOT NULL,\n    full_name TEXT NOT NULL,\n    webhook_secret_env_ref TEXT NOT NULL,\n    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    UNIQUE (provider, full_name)\n);\n\nCREATE TABLE IF NOT EXISTS issues (\n    id UUID PRIMARY KEY,\n    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,\n    number BIGINT NOT NULL,\n    title TEXT NOT NULL,\n    state TEXT NOT NULL,\n    status TEXT NOT NULL,\n    labels JSONB NOT NULL DEFAULT '[]'::jsonb,\n    assignee TEXT NOT NULL DEFAULT '',\n    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    UNIQUE (repository_id, number)\n);\n\nCREATE TABLE IF NOT EXISTS pull_requests (\n    id UUID PRIMARY KEY,\n    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,\n    number BIGINT NOT NULL,\n    title TEXT NOT NULL,\n    state TEXT NOT NULL,\n    head_sha TEXT NOT NULL,\n    branch TEXT NOT NULL,\n    closes JSONB NOT NULL DEFAULT '[]'::jsonb,\n    merged_at TIMESTAMPTZ,\n    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    UNIQUE (repository_id, number)\n);\n\nCREATE TABLE IF NOT EXISTS pipeline_runs (\n    id UUID PRIMARY KEY,\n    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,\n    external_id TEXT NOT NULL,\n    pr_number BIGINT,\n    sha TEXT NOT NULL,\n    status TEXT NOT NULL,\n    conclusion TEXT NOT NULL,\n    started_at TIMESTAMPTZ,\n    finished_at TIMESTAMPTZ,\n    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    UNIQUE (repository_id, external_id)\n);\n\nCREATE TABLE IF NOT EXISTS webhook_events (\n    id UUID PRIMARY KEY,\n    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,\n    provider TEXT NOT NULL,\n    event_type TEXT NOT NULL,\n    delivery_id TEXT NOT NULL,\n    payload JSONB NOT NULL DEFAULT '{}'::jsonb,\n    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),\n    UNIQUE (repository_id, provider, delivery_id)\n);\n\nCREATE INDEX IF NOT EXISTS idx_issues_repository_updated ON issues(repository_id, updated_at DESC);\nCREATE INDEX IF NOT EXISTS idx_pull_requests_repository_updated ON pull_requests(repository_id, updated_at DESC);\nCREATE INDEX IF NOT EXISTS idx_pipeline_runs_repository_sha ON pipeline_runs(repository_id, sha);\nCREATE INDEX IF NOT EXISTS idx_webhook_events_repository_received ON webhook_events(repository_id, received_at DESC)\n"

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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(storeSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).
		WithArgs(2, "create_store").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).WithArgs(3).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(`ALTER TABLE issues ADD COLUMN IF NOT EXISTS upstream_updated_at`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(3, "add_upstream_updated_at").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).WithArgs(4).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE agent_runs (")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`)).WithArgs(4, "create_agent_runs").WillReturnResult(sqlmock.NewResult(1, 1))
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
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).
		WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "schema_migrations" WHERE version = $1`)).WithArgs(4).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

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
