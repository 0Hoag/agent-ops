package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
)

type Config = scm.Config

type Provider struct {
	token, secret, baseURL string
	client                 *http.Client
	mappings               map[string]string
}

func New(config Config) *Provider {
	base := strings.TrimRight(config.APIBaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{token: config.Token, secret: config.WebhookSecret, baseURL: base, client: client, mappings: config.StatusMappings}
}

func init() {
	_ = builder.Register("github", func(config scm.Config) scm.SCMProvider { return New(config) })
}

func VerifySignature(secret string, payload []byte, signature string) error {
	if secret == "" || !strings.HasPrefix(signature, "sha256=") {
		return errors.New("invalid GitHub signature")
	}
	expected, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil || len(expected) != sha256.Size {
		return errors.New("invalid GitHub signature")
	}
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(payload)
	if !hmac.Equal(expected, digest.Sum(nil)) {
		return errors.New("invalid GitHub signature")
	}
	return nil
}

func (p *Provider) VerifyWebhook(ctx context.Context, headers http.Header, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return VerifySignature(p.secret, payload, headers.Get("X-Hub-Signature-256"))
}
