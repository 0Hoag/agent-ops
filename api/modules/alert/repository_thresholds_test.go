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

func thresholdDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

func TestRepositoryUpsertThresholdsUsesOrganizationConflictAndSeconds(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000001"
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO alert_thresholds (organization_id, min_cache_hit, silent_seconds, stale_seconds)
		VALUES ($1, $2, $3, $4) ON CONFLICT (organization_id) DO UPDATE SET
		min_cache_hit = EXCLUDED.min_cache_hit, silent_seconds = EXCLUDED.silent_seconds, stale_seconds = EXCLUDED.stale_seconds, updated_at = NOW()`)).
		WithArgs(org, 0.8, int64(60), int64(3600)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, NewRepository(db).UpsertThresholds(context.Background(), org, Thresholds{MinCacheHit: .8, SilentAfter: time.Minute, PRStaleAfter: time.Hour}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryGetThresholdsIsOrganizationScopedAndMapsSeconds(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000001"
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "alert_thresholds" WHERE organization_id = $1 ORDER BY "alert_thresholds"."organization_id" LIMIT $2`)).
		WithArgs(org, 1).WillReturnRows(sqlmock.NewRows([]string{"organization_id", "min_cache_hit", "silent_seconds", "stale_seconds"}).AddRow(org, .75, 90, 7200))
	got, err := NewRepository(db).GetThresholds(context.Background(), org)
	require.NoError(t, err)
	require.Equal(t, &Thresholds{MinCacheHit: .75, SilentAfter: 90 * time.Second, PRStaleAfter: 2 * time.Hour}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryGetThresholdsReturnsNilWhenAbsent(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000002"
	mock.ExpectQuery(`SELECT \* FROM "alert_thresholds"`).WithArgs(org, 1).WillReturnError(gorm.ErrRecordNotFound)
	got, err := NewRepository(db).GetThresholds(context.Background(), org)
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryThresholdsPropagatesDatabaseErrors(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000003"
	mock.ExpectQuery(`SELECT \* FROM "alert_thresholds"`).WithArgs(org, 1).WillReturnError(sqlmock.ErrCancelled)
	_, err := NewRepository(db).GetThresholds(context.Background(), org)
	require.ErrorIs(t, err, sqlmock.ErrCancelled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryThresholdsRejectsInvalidValuesWithoutDatabaseAccess(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000004"
	for _, thresholds := range []Thresholds{{MinCacheHit: -1, SilentAfter: time.Second, PRStaleAfter: time.Second}, {MinCacheHit: 1.1, SilentAfter: time.Second, PRStaleAfter: time.Second}, {MinCacheHit: .5, SilentAfter: 0, PRStaleAfter: time.Second}, {MinCacheHit: .5, SilentAfter: 500 * time.Millisecond, PRStaleAfter: time.Second}} {
		require.ErrorIs(t, NewRepository(db).UpsertThresholds(context.Background(), org, thresholds), ErrInvalidThresholds)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
