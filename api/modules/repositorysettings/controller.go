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
func (c *Controller) view(r *store.Repository) View {
	base := c.webhookBase
	v := View{ID: string(r.ID), Provider: r.Provider, FullName: r.FullName, GitLabBaseURL: r.GitLabBaseURL, WebhookURL: base + "/api/v1/webhooks/" + r.Provider}
	_ = json.Unmarshal(r.StatusMappings, &v.StatusMappings)
	return v
}

// List returns repository settings without stored credentials.
// @Summary List repository settings
// @Produce json
// @Success 200 {object} ListResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/repositories [get]
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
		out = append(out, c.view(&rows[i]))
	}
	response.OK(ctx, out)
}

// Get returns repository settings without stored credentials.
// @Summary Get repository settings
// @Produce json
// @Param repository_id path string true "Repository UUID"
// @Success 200 {object} ViewResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/repositories/{repository_id} [get]
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
	response.OK(ctx, c.view(r))
}

// Create stores repository settings and returns credentials exactly once.
// @Summary Create repository settings
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Repository settings"
// @Success 201 {object} CreatedResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/repositories [post]
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
	v := CreatedView{View: c.view(r), WebhookSecret: req.WebhookSecret, BackfillToken: req.BackfillToken}
	response.Created(ctx, v)
}

// Update changes repository settings and returns a redacted view.
// @Summary Update repository settings
// @Accept json
// @Produce json
// @Param repository_id path string true "Repository UUID"
// @Param request body UpdateRequest true "Repository settings changes"
// @Success 200 {object} ViewResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/repositories/{repository_id} [patch]
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
	response.OK(ctx, c.view(r))
}

// Delete removes repository settings.
// @Summary Delete repository settings
// @Produce json
// @Param repository_id path string true "Repository UUID"
// @Success 204 {object} response.MessageResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/repositories/{repository_id} [delete]
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
