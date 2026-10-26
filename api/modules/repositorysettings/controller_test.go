package repositorysettings

import (
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

func TestViewRedactsStoredCredentialsOnSubsequentReads(t *testing.T) {
	c := &Controller{webhookBase: "https://agent.example"}
	r := &store.Repository{
		ID: "11111111-1111-1111-1111-111111111111", Provider: "github", FullName: "acme/app",
		WebhookSecretCiphertext: "cipher-secret", BackfillTokenCiphertext: "cipher-token",
		StatusMappings: store.JSONB(`{"status:review":"review"}`),
	}
	v := c.view(r, false)
	if v.WebhookSecret != "" || v.BackfillToken != "" {
		t.Fatalf("stored credentials leaked in read view: %+v", v)
	}
	if v.WebhookURL != "https://agent.example/api/v1/webhooks/github" {
		t.Fatalf("unexpected webhook URL %q", v.WebhookURL)
	}
	if v.StatusMappings["status:review"] != "review" {
		t.Fatalf("status mappings missing: %#v", v.StatusMappings)
	}
}
