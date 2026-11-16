package alert

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ALERT_TEST_DATABASE_URL must name an owned disposable PostgreSQL database.
func incidentPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ALERT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ALERT_TEST_DATABASE_URL for real PostgreSQL acceptance")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	schema := "incident_" + uuid.NewString()[:8]
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlAdmin, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, admin.Exec("DROP SCHEMA IF EXISTS "+schema+" CASCADE").Error)
		require.NoError(t, sqlAdmin.Close())
	})
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	query := parsed.Query()
	query.Set("search_path", schema)
	query.Set("application_name", schema)
	parsed.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.Exec(`CREATE TABLE organizations (id UUID PRIMARY KEY)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO organizations VALUES ('00000000-0000-0000-0000-000000000001')`).Error)
	// Load the production occurrence and incident schemas, including removal of uniqueness.
	for _, name := range []string{"015_create_alert_history.sql", "022_alert_incident_state.sql"} {
		data, readErr := os.ReadFile("../../migrations/" + name)
		require.NoError(t, readErr)
		require.NoError(t, db.Exec(string(data)).Error)
	}
	return db
}

func incidentInput() Alert {
	return Alert{OrganizationID: "00000000-0000-0000-0000-000000000001", Key: "same", Repository: "repo", Rule: "rule", Subject: "subject", Message: "message", DetectedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func TestRecordPostgresOverlappingAcquisitions(t *testing.T) {
	db := incidentPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	pool, err := db.DB()
	require.NoError(t, err)
	blocker, err := pool.Conn(ctx)
	require.NoError(t, err)
	defer func() {
		_, _ = blocker.ExecContext(context.Background(), `SELECT pg_advisory_unlock(607)`)
		_ = blocker.Close()
	}()
	_, err = blocker.ExecContext(ctx, `SELECT pg_advisory_lock(607)`)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE FUNCTION pause_occurrence() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(607); RETURN NEW; END $$;
 CREATE TRIGGER pause_occurrence BEFORE INSERT ON alert_occurrences FOR EACH ROW EXECUTE FUNCTION pause_occurrence()`).Error)
	type result struct {
		row      Occurrence
		inserted bool
		err      error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			row, inserted, recordErr := NewRepository(db).Record(ctx, incidentInput())
			results <- result{row, inserted, recordErr}
		}()
	}
	// Both calls must overlap while blocked: the fixed implementation waits on the
	// acquisition lock; the control waits in both occurrence INSERT triggers.
	require.Eventually(t, func() bool {
		var count int64
		err = db.Raw(`SELECT count(*) FROM pg_stat_activity WHERE application_name = current_setting('application_name') AND wait_event_type = 'Lock'`).Scan(&count).Error
		return err == nil && count == 2
	}, 5*time.Second, 10*time.Millisecond)
	_, err = blocker.ExecContext(ctx, `SELECT pg_advisory_unlock(607)`)
	require.NoError(t, err)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	var count int64
	require.NoError(t, db.Table("alert_occurrences").Count(&count).Error)
	require.EqualValues(t, 1, count, "overlapping acquisition must commit one occurrence")
	require.Equal(t, first.row.ID, second.row.ID)
	require.NotEqual(t, first.inserted, second.inserted)
	retry, inserted, err := NewRepository(db).Record(ctx, incidentInput())
	require.NoError(t, err)
	require.False(t, inserted)
	require.Equal(t, first.row.ID, retry.ID)
	require.NoError(t, NewRepository(db).ReconcileStates(ctx, []IncidentState{{Key: "same", Active: false}}, incidentInput().OrganizationID))
	recovered, inserted, err := NewRepository(db).Record(ctx, incidentInput())
	require.NoError(t, err)
	require.True(t, inserted)
	require.NotEqual(t, first.row.ID, recovered.ID)
	retry, inserted, err = NewRepository(db).Record(ctx, incidentInput())
	require.NoError(t, err)
	require.False(t, inserted)
	require.Equal(t, recovered.ID, retry.ID)
}

func TestRecordPostgresActivationFailureRollsBack(t *testing.T) {
	db := incidentPostgres(t)
	require.NoError(t, db.Exec(`CREATE FUNCTION fail_activation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected activation failure'; END $$;
 CREATE TRIGGER fail_activation BEFORE INSERT OR UPDATE ON alert_incident_states FOR EACH ROW EXECUTE FUNCTION fail_activation()`).Error)
	_, inserted, err := NewRepository(db).Record(context.Background(), incidentInput())
	require.ErrorContains(t, err, "injected activation failure")
	require.False(t, inserted)
	var count int64
	require.NoError(t, db.Table("alert_occurrences").Count(&count).Error)
	require.EqualValues(t, 0, count, fmt.Sprintf("failed activation must roll back occurrence (count=%d)", count))
}
