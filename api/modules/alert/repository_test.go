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
	mock.ExpectExec(regexp.QuoteMeta("ON CONFLICT (organization_id, alert_key) DO NOTHING")).WithArgs(sqlmock.AnyArg(), "org-a", "same", "repo", "rule", "subject", "message", at).
		WillReturnResult(sqlmock.NewResult(0, 0))
	_, inserted, err := repo.Record(context.Background(), Alert{Key: "same", OrganizationID: "org-a", Repository: "repo",
		Rule: "rule", Subject: "subject", Message: "message", DetectedAt: at})
	require.NoError(t, err)
	require.False(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecordKeepsDistinctOccurrencesAndDeliveryUpsert(t *testing.T) {
	db, mock := alertDB(t)
	repo := NewRepository(db)
	at := time.Now()
	insert := regexp.QuoteMeta("ON CONFLICT (organization_id, alert_key) DO NOTHING")
	mock.ExpectExec(insert).WithArgs(sqlmock.AnyArg(), "org-a", "first", "repo", "rule", "subject", "message", at).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(insert).WithArgs(sqlmock.AnyArg(), "org-a", "second", "repo", "rule", "subject", "message", at).WillReturnResult(sqlmock.NewResult(0, 1))
	first, inserted, err := repo.Record(context.Background(), Alert{Key: "first", OrganizationID: "org-a", Repository: "repo", Rule: "rule", Subject: "subject", Message: "message", DetectedAt: at})
	require.NoError(t, err)
	require.True(t, inserted)
	_, inserted, err = repo.Record(context.Background(), Alert{Key: "second", OrganizationID: "org-a", Repository: "repo", Rule: "rule", Subject: "subject", Message: "message", DetectedAt: at})
	require.NoError(t, err)
	require.True(t, inserted)
	delivery := regexp.QuoteMeta("ON CONFLICT (occurrence_id, channel) DO UPDATE SET")
	mock.ExpectExec(delivery).WithArgs(sqlmock.AnyArg(), first.ID, "email", true, "", at).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(delivery).WithArgs(sqlmock.AnyArg(), first.ID, "email", false, "failed", at).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.RecordDelivery(context.Background(), Delivery{OccurrenceID: first.ID, Channel: "email", Success: true, DeliveredAt: at}))
	require.NoError(t, repo.RecordDelivery(context.Background(), Delivery{OccurrenceID: first.ID, Channel: "email", Error: "failed", DeliveredAt: at}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListBindsOrganizationAndPagination(t *testing.T) {
	db, mock := alertDB(t)
	repo := NewRepository(db)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "alert_occurrences" WHERE organization_id = $1`)).WithArgs("org-a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "alert_occurrences" WHERE organization_id = $1 ORDER BY detected_at DESC, id DESC LIMIT $2 OFFSET $3`)).WithArgs("org-a", 2, 2).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "alert_key", "repository", "rule", "subject", "message", "detected_at"}))
	rows, total, err := repo.List(context.Background(), "org-a", 2, 2)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.EqualValues(t, 3, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
