package alert

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type channelRequest struct {
	Kind                  string `json:"kind"`
	Enabled               bool   `json:"enabled"`
	TelegramTokenEnvRef   string `json:"telegram_token_env_ref"`
	TelegramChatIDEnvRef  string `json:"telegram_chat_id_env_ref"`
	SlackWebhookURLEnvRef string `json:"slack_webhook_url_env_ref"`
	SMTPHostEnvRef        string `json:"smtp_host_env_ref"`
	SMTPPortEnvRef        string `json:"smtp_port_env_ref"`
	SMTPUsernameEnvRef    string `json:"smtp_username_env_ref"`
	SMTPPasswordEnvRef    string `json:"smtp_password_env_ref"`
	SMTPFromEnvRef        string `json:"smtp_from_env_ref"`
	SMTPToEnvRef          string `json:"smtp_to_env_ref"`
}

type channelResponse struct {
	Kind                  string `json:"kind"`
	Enabled               bool   `json:"enabled"`
	TelegramTokenEnvRef   string `json:"telegram_token_env_ref,omitempty"`
	TelegramChatIDEnvRef  string `json:"telegram_chat_id_env_ref,omitempty"`
	SlackWebhookURLEnvRef string `json:"slack_webhook_url_env_ref,omitempty"`
	SMTPHostEnvRef        string `json:"smtp_host_env_ref,omitempty"`
	SMTPPortEnvRef        string `json:"smtp_port_env_ref,omitempty"`
	SMTPUsernameEnvRef    string `json:"smtp_username_env_ref,omitempty"`
	SMTPPasswordEnvRef    string `json:"smtp_password_env_ref,omitempty"`
	SMTPFromEnvRef        string `json:"smtp_from_env_ref,omitempty"`
	SMTPToEnvRef          string `json:"smtp_to_env_ref,omitempty"`
}

var envReference = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func (r channelRequest) channel(org, kind string) Channel {
	return Channel{OrganizationID: org, Kind: kind, Enabled: r.Enabled, TelegramTokenEnvRef: r.TelegramTokenEnvRef, TelegramChatIDEnvRef: r.TelegramChatIDEnvRef, SlackWebhookURLEnvRef: r.SlackWebhookURLEnvRef, SMTPHostEnvRef: r.SMTPHostEnvRef, SMTPPortEnvRef: r.SMTPPortEnvRef, SMTPUsernameEnvRef: r.SMTPUsernameEnvRef, SMTPPasswordEnvRef: r.SMTPPasswordEnvRef, SMTPFromEnvRef: r.SMTPFromEnvRef, SMTPToEnvRef: r.SMTPToEnvRef}
}

func (r channelRequest) valid(kind string) bool {
	refs := []string{r.TelegramTokenEnvRef, r.TelegramChatIDEnvRef, r.SlackWebhookURLEnvRef, r.SMTPHostEnvRef, r.SMTPPortEnvRef, r.SMTPUsernameEnvRef, r.SMTPPasswordEnvRef, r.SMTPFromEnvRef, r.SMTPToEnvRef}
	for _, ref := range refs {
		if ref != "" && !envReference.MatchString(ref) {
			return false
		}
	}
	switch kind {
	case "telegram":
		return r.TelegramTokenEnvRef != "" && r.TelegramChatIDEnvRef != ""
	case "slack":
		return r.SlackWebhookURLEnvRef != ""
	case "smtp":
		return r.SMTPHostEnvRef != "" && r.SMTPPortEnvRef != "" && r.SMTPUsernameEnvRef != "" && r.SMTPPasswordEnvRef != "" && r.SMTPFromEnvRef != "" && r.SMTPToEnvRef != ""
	default:
		return false
	}
}

func channelView(c Channel) channelResponse {
	return channelResponse{Kind: c.Kind, Enabled: c.Enabled, TelegramTokenEnvRef: c.TelegramTokenEnvRef, TelegramChatIDEnvRef: c.TelegramChatIDEnvRef, SlackWebhookURLEnvRef: c.SlackWebhookURLEnvRef, SMTPHostEnvRef: c.SMTPHostEnvRef, SMTPPortEnvRef: c.SMTPPortEnvRef, SMTPUsernameEnvRef: c.SMTPUsernameEnvRef, SMTPPasswordEnvRef: c.SMTPPasswordEnvRef, SMTPFromEnvRef: c.SMTPFromEnvRef, SMTPToEnvRef: c.SMTPToEnvRef}
}

func (c *Controller) channelOrganization(ctx *gin.Context, write bool) (string, bool) {
	org, ok := c.thresholdOrganization(ctx, write)
	return org, ok
}

// @Summary List alert channels
// @Tags alerts
// @Produce json
// @Success 200 {array} channelResponse
// @Router /api/v1/alerts/channels [get]
func (c *Controller) ListChannels(ctx *gin.Context) {
	org, ok := c.channelOrganization(ctx, false)
	if !ok {
		return
	}
	rows, err := c.repository.ListChannels(ctx, org)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to list alert channels")
		return
	}
	out := make([]channelResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, channelView(row))
	}
	response.OK(ctx, out)
}

// @Summary Create alert channel
// @Tags alerts
// @Accept json
// @Produce json
// @Param channel body channelRequest true "Environment references"
// @Success 201 {object} channelResponse
// @Router /api/v1/alerts/channels [post]
func (c *Controller) CreateChannel(ctx *gin.Context) {
	c.writeChannel(ctx, false)
}

// @Summary Update alert channel
// @Tags alerts
// @Accept json
// @Produce json
// @Param kind path string true "Channel kind"
// @Param channel body channelRequest true "Environment references"
// @Success 200 {object} channelResponse
// @Router /api/v1/alerts/channels/{kind} [put]
func (c *Controller) UpdateChannel(ctx *gin.Context) {
	c.writeChannel(ctx, true)
}

func (c *Controller) writeChannel(ctx *gin.Context, update bool) {
	org, ok := c.channelOrganization(ctx, true)
	if !ok {
		return
	}
	var req channelRequest
	if ctx.ShouldBindJSON(&req) != nil {
		response.Error(ctx, http.StatusBadRequest, "invalid alert channel")
		return
	}
	kind := strings.ToLower(strings.TrimSpace(ctx.Param("kind")))
	if kind == "" {
		kind = strings.ToLower(strings.TrimSpace(req.Kind))
	}
	if !req.valid(kind) {
		response.Error(ctx, http.StatusBadRequest, "invalid alert channel")
		return
	}
	channel := req.channel(org, kind)
	var err error
	if update {
		err = c.repository.UpdateChannel(ctx, channel)
	} else {
		err = c.repository.CreateChannel(ctx, channel)
	}
	if err != nil {
		if errors.Is(err, ErrInvalidChannel) {
			response.Error(ctx, http.StatusBadRequest, "invalid alert channel")
			return
		}
		response.Error(ctx, http.StatusInternalServerError, "failed to save alert channel")
		return
	}
	if update {
		response.OK(ctx, channelView(channel))
	} else {
		response.Created(ctx, channelView(channel))
	}
}

// @Summary Delete alert channel
// @Tags alerts
// @Produce json
// @Param kind path string true "Channel kind"
// @Success 200 {object} response.MessageResponse
// @Router /api/v1/alerts/channels/{kind} [delete]
func (c *Controller) DeleteChannel(ctx *gin.Context) {
	org, ok := c.channelOrganization(ctx, true)
	if !ok {
		return
	}
	kind := strings.ToLower(strings.TrimSpace(ctx.Param("kind")))
	if err := c.repository.DeleteChannel(ctx, org, kind); err != nil {
		if errors.Is(err, ErrInvalidChannel) {
			response.Error(ctx, http.StatusBadRequest, "invalid alert channel")
			return
		}
		response.Error(ctx, http.StatusInternalServerError, "failed to delete alert channel")
		return
	}
	response.OKMessage(ctx, "alert channel deleted")
}
