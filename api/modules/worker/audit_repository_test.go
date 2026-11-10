package worker

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAuditRepositoryUsesMigratedTableMapping(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{})
	require.NoError(t, err)

	audit := &ActionAudit{
		ID:             store.NewUUID(),
		OrganizationID: "org-1",
		ActorID:        "actor-1",
		OccurredAt:     time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
		Action:         "start",
		Wave:           "W17",
		JobID:          "job-1",
		Outcome:        "succeeded",
		CreatedAt:      time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
	}
	mock.ExpectBegin()
	// All fields, including the primary key, are supplied; GORM executes without RETURNING.
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "worker_action_audits"`)).
		WithArgs(audit.ID, audit.OrganizationID, audit.ActorID, audit.OccurredAt, audit.Action, audit.Wave, audit.JobID, audit.Outcome, audit.CreatedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := &auditRepository{db: db}
	require.NoError(t, repo.Create(context.Background(), audit))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "worker_action_audits"`)).
		WithArgs(audit.OrganizationID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "worker_action_audits" WHERE organization_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT $2`)).
		WithArgs(audit.OrganizationID, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "actor_id", "occurred_at", "action", "wave", "job_id", "outcome", "created_at"}).
			AddRow(audit.ID, audit.OrganizationID, audit.ActorID, audit.OccurredAt, audit.Action, audit.Wave, audit.JobID, audit.Outcome, audit.CreatedAt))

	rows, total, err := repo.List(context.Background(), audit.OrganizationID, 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, audit.ID, rows[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}
