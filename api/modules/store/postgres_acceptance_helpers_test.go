package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetAcceptanceRows(t *testing.T, db *gorm.DB, repositoryID UUID) {
	t.Helper()
	for _, table := range []string{"issues", "pull_requests", "pipeline_runs"} {
		require.NoError(t, db.Exec(fmt.Sprintf("DELETE FROM %s WHERE repository_id = ?", table), repositoryID).Error)
	}
}
