package docs

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/swaggo/swag"
)

func TestGeneratedSwaggerIsRegisteredWithWebhookPath(t *testing.T) {
	spec, err := swag.ReadDoc("swagger")
	require.NoError(t, err)
	require.Contains(t, spec, "/api/v1/webhooks/{provider}")
}
