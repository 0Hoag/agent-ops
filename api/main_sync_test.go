package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/0Hoag/agent-ops/api/pkg/config"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
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

func TestConfiguredRepositorySealsStartupCredentials(t *testing.T) {
	t.Setenv("STARTUP_TOKEN", "bearer-test-secret")
	t.Setenv("STARTUP_SECRET", "webhook-test-secret")
	sealer, err := appcrypto.New([]byte("01234567890123456789012345678901"))
	require.NoError(t, err)

	repository, err := configuredRepository(config.RepositoryConfig{Provider: "github", FullName: "acme/app", OrganizationID: "org-id", TokenEnv: "STARTUP_TOKEN", WebhookSecretEnv: "STARTUP_SECRET"}, sealer)
	require.NoError(t, err)
	require.NotEqual(t, "bearer-test-secret", repository.BackfillTokenCiphertext)
	require.NotEqual(t, "webhook-test-secret", repository.WebhookSecretCiphertext)
	token, err := sealer.Open(repository.BackfillTokenCiphertext)
	require.NoError(t, err)
	require.Equal(t, "bearer-test-secret", token)
	secret, err := sealer.Open(repository.WebhookSecretCiphertext)
	require.NoError(t, err)
	require.Equal(t, "webhook-test-secret", secret)
}

func TestConfiguredRepositoryMapsStartupBaseURL(t *testing.T) {
	t.Setenv("STARTUP_BASE_URL", "https://gitlab.synthetic.test")

	repository, err := configuredRepository(config.RepositoryConfig{
		Provider:       "gitlab",
		FullName:       "fixture/group/scheduled",
		OrganizationID: "org-id",
		BaseURLEnv:     "STARTUP_BASE_URL",
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "https://gitlab.synthetic.test", repository.GitLabBaseURL)
}
