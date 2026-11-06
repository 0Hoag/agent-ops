package alert

import (
	"net/http"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/pkg/middleware"
	"github.com/0Hoag/agent-ops/api/pkg/pagination"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Controller struct {
	repository *Repository
	orgs       *organization.Repository
}

type HistoryResponse struct {
	Data []Occurrence                 `json:"data"`
	Meta pagination.PaginatorResponse `json:"meta"`
}

func NewController(repository *Repository, orgs ...*organization.Repository) *Controller {
	c := &Controller{repository: repository}
	if len(orgs) > 0 {
		c.orgs = orgs[0]
	}
	return c
}

type ThresholdsResponse struct {
	MinCacheHit    float64 `json:"min_cache_hit"`
	SilentSeconds  int64   `json:"silent_seconds"`
	PRStaleSeconds int64   `json:"pr_stale_seconds"`
}

func thresholdResponse(v Thresholds) ThresholdsResponse {
	return ThresholdsResponse{MinCacheHit: v.MinCacheHit, SilentSeconds: int64(v.SilentAfter / time.Second), PRStaleSeconds: int64(v.PRStaleAfter / time.Second)}
}

func (c *Controller) thresholdOrganization(ctx *gin.Context, write bool) (string, bool) {
	org, ok := middleware.OrganizationID(ctx)
	if !ok || c.orgs == nil {
		response.Unauthorized(ctx, "organization selection required")
		return "", false
	}
	account, exists := ctx.Get("account_id")
	accountID, valid := account.(string)
	if !exists || !valid || accountID == "" {
		response.Unauthorized(ctx, "authentication required")
		return "", false
	}
	m, err := c.orgs.Membership(ctx.Request.Context(), org, accountID)
	if err != nil {
		response.Forbidden(ctx, "organization access denied")
		return "", false
	}
	if write && m.Role != organization.RoleOwner && m.Role != organization.RoleAdmin {
		response.Forbidden(ctx, "alert thresholds require owner or admin")
		return "", false
	}
	return org, true
}

// History lists persisted alert occurrences for the authenticated organization.
// @Summary List alert history
// @Produce json
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Success 200 {object} HistoryResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/alerts/history [get]
func (c *Controller) History(ctx *gin.Context) {
	org, ok := middleware.OrganizationID(ctx)
	if !ok {
		response.Unauthorized(ctx, "organization selection required")
		return
	}
	p := pagination.Parse(ctx)
	rows, total, err := c.repository.List(ctx.Request.Context(), org, p.Page, p.PerPage)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to list alert history")
		return
	}
	response.JSON(ctx, http.StatusOK, HistoryResponse{Data: rows, Meta: p.ToResponse(total)})
}
