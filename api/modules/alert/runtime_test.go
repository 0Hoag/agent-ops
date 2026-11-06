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
