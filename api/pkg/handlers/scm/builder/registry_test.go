package builder

import (
	"context"
	"net/http"
	"testing"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct{}

func (fakeProvider) VerifyWebhook(context.Context, http.Header, []byte) error { return nil }
func (fakeProvider) ParseEvent(context.Context, string, http.Header, []byte) (scm.Event, error) {
	return scm.Event{}, nil
}

func TestRegistryPassesConfigurationToFactories(t *testing.T) {
	registry := NewRegistry()
	var got []scm.Config
	require.NoError(t, registry.Register("configured", func(config scm.Config) scm.SCMProvider {
		got = append(got, config)
		return fakeProvider{}
	}))
	_, err := registry.Build("configured", scm.Config{Token: "one", WebhookSecret: "secret-one"})
	require.NoError(t, err)
	_, err = registry.Build("configured", scm.Config{Token: "two", WebhookSecret: "secret-two"})
	require.NoError(t, err)
	require.Equal(t, []scm.Config{{Token: "one", WebhookSecret: "secret-one"}, {Token: "two", WebhookSecret: "secret-two"}}, got)
}
func (fakeProvider) Backfill(context.Context, string) (scm.BackfillResult, error) {
	return scm.BackfillResult{}, nil
}

func TestRegistryRejectsUnknownAndDuplicate(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register("fake", func(scm.Config) scm.SCMProvider { return fakeProvider{} }))
	require.Error(t, registry.Register("FAKE", func(scm.Config) scm.SCMProvider { return fakeProvider{} }))
	_, err := registry.Build("missing", scm.Config{})
	require.Error(t, err)
	provider, err := registry.Build("FAKE", scm.Config{})
	require.NoError(t, err)
	require.NotNil(t, provider)
}
