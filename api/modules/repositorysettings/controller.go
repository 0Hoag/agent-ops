package repositorysettings

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Controller struct {
	repos       *store.RepositoryRepository
	orgs        *organization.Repository
	sealer      *crypto.Sealer
	webhookBase string
}

func NewController(repos *store.RepositoryRepository, orgs *organization.Repository, sealer *crypto.Sealer, webhookBase string) *Controller {
	return &Controller{repos: repos, orgs: orgs, sealer: sealer, webhookBase: strings.TrimRight(webhookBase, "/")}
}
func (c *Controller) org(ctx *gin.Context) (string, bool) {
	id, ok := ctx.Get("organization_id")
	s, valid := id.(string)
	if !ok || !valid || s == "" {
		response.Unauthorized(ctx, "organization required")
		return "", false
	}
	return s, true
}
func (c *Controller) allowed(ctx *gin.Context, write bool) (string, bool) {
	org, ok := c.org(ctx)
	if !ok {
		return "", false
	}
	p, err := c.orgs.Membership(ctx.Request.Context(), org, accountID(ctx))
	if err != nil {
		response.Forbidden(ctx, "organization access denied")
		return "", false
	}
	if write && p.Role != "owner" && p.Role != "admin" {
		response.Forbidden(ctx, "repository settings require owner or admin")
		return "", false
	}
	return org, true
}
func accountID(ctx *gin.Context) string { v, _ := ctx.Get("account_id"); s, _ := v.(string); return s }
func (c *Controller) view(r *store.Repository, reveal bool) View {
	base := c.webhookBase
	v := View{ID: string(r.ID), Provider: r.Provider, FullName: r.FullName, GitLabBaseURL: r.GitLabBaseURL, WebhookURL: base + "/api/v1/webhooks/" + r.Provider}
	_ = json.Unmarshal(r.StatusMappings, &v.StatusMappings)
	if reveal {
		v.WebhookSecret = ""
		v.BackfillToken = ""
	}
	return v
}
func (c *Controller) List(ctx *gin.Context) {
	org, ok := c.allowed(ctx, false)
	if !ok {
		return
	}
	rows, err := c.repos.ListByOrganization(ctx, org)
	if err != nil {
		response.InternalError(ctx, "failed to list repositories")
		return
	}
	out := make([]View, 0, len(rows))
	for i := range rows {
		out = append(out, c.view(&rows[i], false))
	}
	response.OK(ctx, out)
}
func (c *Controller) Get(ctx *gin.Context) {
	org, ok := c.allowed(ctx, false)
	if !ok {
		return
	}
	r, err := c.repos.GetByOrganization(ctx, store.UUID(org), store.UUID(ctx.Param("repository_id")))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(ctx, "repository not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to get repository")
		return
	}
	response.OK(ctx, c.view(r, false))
}
func (c *Controller) Create(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	var req CreateRequest
	if ctx.ShouldBindJSON(&req) != nil || req.Provider == "" || req.FullName == "" || c.sealer == nil {
		response.BadRequest(ctx, "invalid request")
		return
	}
	secret, err := c.sealer.Seal(req.WebhookSecret)
	if err != nil {
		response.InternalError(ctx, "failed to encrypt credentials")
		return
	}
	token, err := c.sealer.Seal(req.BackfillToken)
	if err != nil {
		response.InternalError(ctx, "failed to encrypt credentials")
		return
	}
	mapping, _ := json.Marshal(req.StatusMappings)
	r := &store.Repository{ID: store.NewUUID(), OrganizationID: org, Provider: req.Provider, FullName: req.FullName, WebhookSecretEnvRef: "", WebhookSecretCiphertext: secret, BackfillTokenCiphertext: token, GitLabBaseURL: req.GitLabBaseURL, StatusMappings: store.JSONB(mapping)}
	if err = c.repos.Create(ctx, r); err != nil {
		response.InternalError(ctx, "failed to create repository")
		return
	}
	v := c.view(r, true)
	v.WebhookSecret = req.WebhookSecret
	v.BackfillToken = req.BackfillToken
	response.Created(ctx, v)
}
func (c *Controller) Update(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	var req UpdateRequest
	if ctx.ShouldBindJSON(&req) != nil {
		response.BadRequest(ctx, "invalid request")
		return
	}
	r, err := c.repos.GetByOrganization(ctx, store.UUID(org), store.UUID(ctx.Param("repository_id")))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(ctx, "repository not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to get repository")
		return
	}
	if req.WebhookSecret != nil {
		r.WebhookSecretCiphertext, err = c.sealer.Seal(*req.WebhookSecret)
	}
	if err == nil && req.BackfillToken != nil {
		r.BackfillTokenCiphertext, err = c.sealer.Seal(*req.BackfillToken)
	}
	if err != nil {
		response.InternalError(ctx, "failed to encrypt credentials")
		return
	}
	if req.GitLabBaseURL != nil {
		r.GitLabBaseURL = *req.GitLabBaseURL
	}
	if req.StatusMappings != nil {
		r.StatusMappings, _ = json.Marshal(req.StatusMappings)
	}
	if err = c.repos.Update(ctx, r); err != nil {
		response.InternalError(ctx, "failed to update repository")
		return
	}
	response.OK(ctx, c.view(r, false))
}
func (c *Controller) Delete(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	err := c.repos.DeleteByOrganization(ctx, org, store.UUID(ctx.Param("repository_id")))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(ctx, "repository not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to delete repository")
		return
	}
	ctx.Status(http.StatusNoContent)
}
