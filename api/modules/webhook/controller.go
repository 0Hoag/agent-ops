package webhook

import (
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"os"
)

type Controller struct {
	service *Service
	configs map[string]Config
}
type Config struct{ SecretEnv, TokenEnv, BaseURLEnv string }

func NewController(service *Service, configs map[string]Config) *Controller {
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
	config, ok := controller.configs[c.Param("provider")]
	if !ok {
		response.Error(c, 404, "unknown provider")
		return
	}
	created, err := controller.service.Ingest(c.Request.Context(), c.Param("provider"), os.Getenv(config.SecretEnv), os.Getenv(config.TokenEnv), os.Getenv(config.BaseURLEnv), c.Request.Header, body)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"accepted": created})
}
