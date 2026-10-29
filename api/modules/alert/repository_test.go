package alert

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

func alertDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

func TestRecordUsesOrganizationAlertConflictKey(t *testing.T) {
	db, mock := alertDB(t)
	repo := NewRepository(db)
	at := time.Now()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO alert_occurrences")).WithArgs(sqlmock.AnyArg(), "org-a", "same", "repo", "rule", "subject", "message", at).
		WillReturnResult(sqlmock.NewResult(0, 0))
	_, inserted, err := repo.Record(context.Background(), Alert{Key: "same", OrganizationID: "org-a", Repository: "repo",
		Rule: "rule", Subject: "subject", Message: "message", DetectedAt: at})
	require.NoError(t, err)
	require.False(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}
