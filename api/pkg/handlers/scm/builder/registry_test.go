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
func (fakeProvider) Backfill(context.Context, string) (scm.BackfillResult, error) {
	return scm.BackfillResult{}, nil
}

func TestRegistryRejectsUnknownAndDuplicate(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register("fake", func() scm.SCMProvider { return fakeProvider{} }))
	require.Error(t, registry.Register("FAKE", func() scm.SCMProvider { return fakeProvider{} }))
	_, err := registry.Build("missing")
	require.Error(t, err)
	provider, err := registry.Build("FAKE")
	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestDefaultRegistryHasGitHub(t *testing.T) {
	provider, err := Build("github")
	require.NoError(t, err)
	require.NotNil(t, provider)
}