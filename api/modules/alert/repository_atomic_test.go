package alert

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRecordRollsBackActivationFailure(t *testing.T) {
	db, mock := alertDB(t)
	input := incidentInput()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(input.OrganizationID, input.Key).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO alert_occurrences`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO alert_incident_states`).WillReturnError(errors.New("activation failed"))
	mock.ExpectRollback()
	row, inserted, err := NewRepository(db).Record(context.Background(), input)
	require.ErrorContains(t, err, "activation failed")
	require.Empty(t, row.ID)
	require.False(t, inserted)
	require.NoError(t, mock.ExpectationsWereMet())
}
