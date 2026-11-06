package alert

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRuntimeNotifierForUsesServerSideEnvironmentAndSkipsIncompleteChannels(t *testing.T) {
	t.Setenv("ALERT_TOKEN", "secret")
	t.Setenv("ALERT_CHAT", "chat")
	notifier, ok := notifierFor(Channel{Kind: "telegram", TelegramTokenEnvRef: "ALERT_TOKEN", TelegramChatIDEnvRef: "ALERT_CHAT"})
	require.True(t, ok)
	require.NotNil(t, notifier)

	_, ok = notifierFor(Channel{Kind: "telegram", TelegramTokenEnvRef: "MISSING_TOKEN", TelegramChatIDEnvRef: "ALERT_CHAT"})
	require.False(t, ok)
}

func TestRuntimeTriggerCancellationStopsBeforeRepositoryScan(t *testing.T) {
	db, mock := thresholdDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewRuntime(db, time.Hour, 10).Trigger(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimeStartPerformsInitialBoundedScan(t *testing.T) {
	db, mock := thresholdDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, full_name FROM "repositories" ORDER BY id LIMIT $1`)).
		WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "full_name"}))

	NewRuntime(db, time.Hour, 10).Start(ctx)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if mock.ExpectationsWereMet() == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimeRepositoryScanAdvancesAcrossTicksAndWraps(t *testing.T) {
	db, mock := alertDB(t)
	runtime := NewRuntime(db, time.Hour, 2)
	rows := func(values ...string) *sqlmock.Rows {
		result := sqlmock.NewRows([]string{"id", "organization_id", "full_name"})
		for i := 0; i < len(values); i += 3 {
			result.AddRow(values[i], values[i+1], values[i+2])
		}
		return result
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, full_name FROM "repositories" ORDER BY id LIMIT $1`)).
		WithArgs(2).WillReturnRows(rows("repo-1", "org-a", "one", "repo-2", "org-a", "two"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, full_name FROM "repositories" WHERE id > $1 ORDER BY id LIMIT $2`)).
		WithArgs("repo-2", 2).WillReturnRows(rows("repo-3", "org-b", "three"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, full_name FROM "repositories" WHERE id > $1 ORDER BY id LIMIT $2`)).
		WithArgs("repo-3", 2).WillReturnRows(rows())
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, full_name FROM "repositories" ORDER BY id LIMIT $1`)).
		WithArgs(2).WillReturnRows(rows("repo-1", "org-a", "one", "repo-2", "org-a", "two"))

	first, err := runtime.nextRepositories(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"repo-1", "repo-2"}, []string{first[0].ID, first[1].ID})
	second, err := runtime.nextRepositories(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"repo-3"}, []string{second[0].ID})
	wrapped, err := runtime.nextRepositories(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"repo-1", "repo-2"}, []string{wrapped[0].ID, wrapped[1].ID})
	require.NoError(t, mock.ExpectationsWereMet())
}
