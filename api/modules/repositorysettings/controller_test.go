package repositorysettings

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

func TestRoutesRegisterAllSettingsOperations(t *testing.T) {
	r := gin.New()
	NewRoutes(&Controller{}).Register(r.Group("/api/v1"))
	routes := r.Routes()
	got := make(map[string]bool, len(routes))
	for _, route := range routes {
		got[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/v1/repositories",
		"POST /api/v1/repositories",
		"GET /api/v1/repositories/:repository_id",
		"PATCH /api/v1/repositories/:repository_id",
		"DELETE /api/v1/repositories/:repository_id",
	} {
		require.True(t, got[route], "missing settings route %s", route)
	}
}

func TestViewRedactsStoredCredentialsOnSubsequentReads(t *testing.T) {
	c := &Controller{webhookBase: "https://agent.example"}
	r := &store.Repository{
		ID: "11111111-1111-1111-1111-111111111111", Provider: "github", FullName: "acme/app",
		WebhookSecretCiphertext: "cipher-secret", BackfillTokenCiphertext: "cipher-token",
		StatusMappings: store.JSONB(`{"status:review":"review"}`),
	}
	v := c.view(r)
	b, err := json.Marshal(v)
	require.NoError(t, err)
	require.NotContains(t, string(b), "cipher-secret")
	require.NotContains(t, string(b), "cipher-token")
	require.NotContains(t, string(b), "webhook_secret")
	require.NotContains(t, string(b), "backfill_token")
	if v.WebhookURL != "https://agent.example/api/v1/webhooks/github" {
		t.Fatalf("unexpected webhook URL %q", v.WebhookURL)
	}
	if v.StatusMappings["status:review"] != "review" {
		t.Fatalf("status mappings missing: %#v", v.StatusMappings)
	}
}
