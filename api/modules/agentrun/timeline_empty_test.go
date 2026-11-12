package agentrun

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestReplaceStepsClearsEmptyTimeline(t *testing.T) {
	for _, steps := range [][]RunStep{nil, {}} {
		t.Run("empty", func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
			require.NoError(t, err)
			const id = "00000000-0000-0000-0000-000000000001"
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT .* FROM agent_runs ar`).WithArgs(id, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
			mock.ExpectExec(`DELETE FROM agent_run_steps WHERE run_id = \$1`).WithArgs(id).
				WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectCommit()
			mock.ExpectClose()
			require.NoError(t, NewRepository(gdb).ReplaceSteps(context.Background(), id, steps))
			require.NoError(t, db.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
