package gitlab

import (
	"context"
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
}

func New(config Config) *Provider {
	base := strings.TrimRight(config.APIBaseURL, "/")
	if base == "" {
		base = "https://gitlab.com"
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{token: config.Token, secret: config.WebhookSecret, baseURL: base, client: client}
}

func init() {
	_ = builder.Register("gitlab", func(config scm.Config) scm.SCMProvider { return New(config) })
}

func VerifyToken(secret, token string) error {
	if secret == "" || token == "" || secret != token {
		return errors.New("invalid GitLab token")
	}
	return nil
}

func (p *Provider) VerifyWebhook(ctx context.Context, headers http.Header, _ []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return VerifyToken(p.secret, headers.Get("X-Gitlab-Token"))
}
