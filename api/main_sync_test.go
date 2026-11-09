package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

type scheduledSyncStub struct{ err error }

func (s scheduledSyncStub) SyncAll(context.Context) error { return s.err }

func TestScheduledSyncDoesNotLogReturnedSecrets(t *testing.T) {
	var output bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&output, nil))
	runScheduledSync(context.Background(), log, scheduledSyncStub{err: errors.New(exposedScheduledSecrets)})
	require.Contains(t, output.String(), "scheduled repository sync failed")
	require.NotContains(t, output.String(), "bearer-test-secret")
	require.NotContains(t, output.String(), "url-password")
	require.NotContains(t, output.String(), "webhook-test-secret")
}

func TestScheduledSyncSuccessDoesNotLogFailure(t *testing.T) {
	var output bytes.Buffer
	runScheduledSync(context.Background(), slog.New(slog.NewJSONHandler(&output, nil)), scheduledSyncStub{})
	require.Empty(t, output.String())
}

const exposedScheduledSecrets = "Bearer bearer-test-secret https://url-user:url-password@example.test hook=webhook-test-secret"
