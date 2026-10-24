package webhook

import (
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
	configs map[string][]Config
}
type Config struct {
	Repository, SecretEnv, TokenEnv, BaseURLEnv string
}

func NewController(service *Service, configs map[string][]Config) *Controller {
	return &Controller{service: service, configs: configs}
}

// Handle receives a signed provider webhook.
// @Summary Ingest provider webhook
// @Router /api/v1/webhooks/{provider} [post]
func (controller *Controller) Handle(c *gin.Context) {
	if c.Request.ContentLength > 2<<20 {
		response.Error(c, http.StatusRequestEntityTooLarge, "payload is too large")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20+1))
	if err != nil || len(body) > 2<<20 {
		response.Error(c, 400, "invalid webhook payload")
		return
	}
	configs := controller.configs[c.Param("provider")]
	if len(configs) == 0 {
		response.Error(c, 404, "unknown provider")
		return
	}
	if len(configs) > 32 {
		response.Error(c, 400, "invalid webhook configuration")
		return
	}
	var created bool
	accepted := false
	for _, config := range configs {
		var verified scm.SCMProvider
		verified, err = controller.service.VerifyConfigured(c.Request.Context(), c.Param("provider"), os.Getenv(config.SecretEnv), os.Getenv(config.TokenEnv), os.Getenv(config.BaseURLEnv), c.Request.Header, body)
		if err == nil {
			created, err = controller.service.IngestParsed(c.Request.Context(), c.Param("provider"), config.Repository, verified, c.Request.Header, body)
		}
		if errors.Is(err, ErrCandidateMismatch) {
			continue
		}
		if err == nil {
			accepted = true
			break
		}
	}
	if !accepted {
		response.Error(c, http.StatusBadRequest, "webhook rejected")
		return
	}
	response.Success(c, http.StatusOK, gin.H{"accepted": created})
}
