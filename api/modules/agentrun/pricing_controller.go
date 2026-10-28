package agentrun

import (
	"context"
	"errors"
	"net/http"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PriceRepository interface {
	ListPrices(context.Context, string) ([]PriceVersion, error)
	CreatePrice(context.Context, *PriceVersion) error
	UpdatePrice(context.Context, string, string, *PriceVersion) error
}
type PriceController struct {
	repo        PriceRepository
	memberships MembershipChecker
}
type MembershipChecker interface {
	Membership(context.Context, string, string) (*organization.Membership, error)
}

func NewPriceController(repo PriceRepository, memberships MembershipChecker) *PriceController {
	return &PriceController{repo: repo, memberships: memberships}
}

func (c *PriceController) allowed(ctx *gin.Context, write bool) (string, bool) {
	org, ok := ctx.Get("organization_id")
	id, ok2 := ctx.Get("account_id")
	if !ok || !ok2 {
		response.Unauthorized(ctx, "organization required")
		return "", false
	}
	member, err := c.memberships.Membership(ctx.Request.Context(), org.(string), id.(string))
	if err != nil {
		response.Forbidden(ctx, "organization access denied")
		return "", false
	}
	if write && member.Role != "owner" && member.Role != "admin" {
		response.Forbidden(ctx, "pricing requires owner or admin")
		return "", false
	}
	return org.(string), true
}

// List returns organization-scoped date-effective model prices.
// @Summary List model price versions
// @Produce json
// @Success 200 {object} response.SuccessResponse
// @Security BearerAuth
// @Router /api/v1/agent-run-prices [get]
// @Accept json
func (c *PriceController) List(ctx *gin.Context) {
	org, ok := c.allowed(ctx, false)
	if !ok {
		return
	}
	v, e := c.repo.ListPrices(ctx, org)
	if e != nil {
		response.InternalError(ctx, "failed to list prices")
		return
	}
	response.OK(ctx, v)
}

// Create adds a date-effective model price version.
// @Summary Create model price version
// @Accept json
// @Produce json
// @Param request body PriceVersion true "Price version"
// @Success 201 {object} response.SuccessResponse
// @Security BearerAuth
// @Router /api/v1/agent-run-prices [post]
func (c *PriceController) Create(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	var p PriceVersion
	if ctx.ShouldBindJSON(&p) != nil {
		response.BadRequest(ctx, "invalid price version")
		return
	}
	p.OrganizationID = store.UUID(org)
	p.ID = store.NewUUID()
	p.EffectiveDate = p.EffectiveDate.UTC()
	if e := validatePrice(&p); e != nil {
		response.BadRequest(ctx, "invalid price version")
		return
	}
	if e := c.repo.CreatePrice(ctx, pointer(p)); e != nil {
		response.Error(ctx, http.StatusConflict, "price version already exists")
		return
	}
	response.Created(ctx, p)
}

// Update changes a date-effective model price version.
// @Summary Update model price version
// @Accept json
// @Produce json
// @Param price_id path string true "Price version UUID"
// @Param request body PriceVersion true "Price version"
// @Success 200 {object} response.SuccessResponse
// @Security BearerAuth
// @Router /api/v1/agent-run-prices/{price_id} [patch]
func (c *PriceController) Update(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	var p PriceVersion
	if ctx.ShouldBindJSON(&p) != nil {
		response.BadRequest(ctx, "invalid price version")
		return
	}
	p.EffectiveDate = p.EffectiveDate.UTC()
	if e := c.repo.UpdatePrice(ctx, org, ctx.Param("price_id"), &p); e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			response.NotFound(ctx, "price version not found")
		} else {
			response.Error(ctx, http.StatusConflict, "price version update failed")
		}
		return
	}
	response.OK(ctx, p)
}
func pointer(p PriceVersion) *PriceVersion { return &p }
