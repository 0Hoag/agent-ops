package budget

import (
	"errors"
	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strings"
)

type Controller struct {
	repo *Repository
	orgs *organization.Repository
}

func NewController(repo *Repository, orgs *organization.Repository) *Controller {
	return &Controller{repo: repo, orgs: orgs}
}
func (c *Controller) allowed(ctx *gin.Context, write bool) (string, bool) {
	ov, ok := ctx.Get("organization_id")
	av, ok2 := ctx.Get("account_id")
	org, ok3 := ov.(string)
	account, ok4 := av.(string)
	if !ok || !ok2 || !ok3 || !ok4 {
		response.Unauthorized(ctx, "organization required")
		return "", false
	}
	m, e := c.orgs.Membership(ctx, org, account)
	if e != nil {
		response.Forbidden(ctx, "organization access denied")
		return "", false
	}
	if write && m.Role != "owner" && m.Role != "admin" {
		response.Forbidden(ctx, "budget requires owner or admin")
		return "", false
	}
	return org, true
}
func valid(req Request) bool {
	return (req.TokenLimit != nil || req.USDLimit != nil) && req.StartsAt.Before(req.EndsAt) && req.StartsAt.IsZero() == false && (req.TokenLimit == nil || *req.TokenLimit > 0) && (req.USDLimit == nil || *req.USDLimit > 0)
}
func (c *Controller) bind(ctx *gin.Context, org string, id string) (*Budget, bool) {
	var req Request
	if ctx.ShouldBindJSON(&req) != nil || !valid(req) {
		response.BadRequest(ctx, "invalid budget")
		return nil, false
	}
	v := &Budget{ID: store.NewUUID(), OrganizationID: store.UUID(org), Wave: strings.TrimSpace(req.Wave), TokenLimit: req.TokenLimit, USDLimit: req.USDLimit, StartsAt: req.StartsAt, EndsAt: req.EndsAt}
	if req.RepositoryID != nil {
		ok, e := c.repo.RepositoryBelongs(ctx, org, *req.RepositoryID)
		if e != nil || !ok {
			response.BadRequest(ctx, "repository does not belong to organization")
			return nil, false
		}
		x := store.UUID(*req.RepositoryID)
		v.RepositoryID = &x
	}
	if id != "" {
		v.ID = store.UUID(id)
	}
	return v, true
}
func (c *Controller) List(ctx *gin.Context) {
	org, ok := c.allowed(ctx, false)
	if !ok {
		return
	}
	v, e := c.repo.List(ctx, org)
	if e != nil {
		response.InternalError(ctx, "failed to list budgets")
		return
	}
	response.OK(ctx, v)
}
func (c *Controller) Get(ctx *gin.Context) {
	org, ok := c.allowed(ctx, false)
	if !ok {
		return
	}
	v, e := c.repo.Get(ctx, org, ctx.Param("budget_id"))
	if errors.Is(e, gorm.ErrRecordNotFound) {
		response.NotFound(ctx, "budget not found")
		return
	}
	if e != nil {
		response.InternalError(ctx, "failed to get budget")
		return
	}
	response.OK(ctx, v)
}
func (c *Controller) Create(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	v, ok := c.bind(ctx, org, "")
	if !ok {
		return
	}
	if e := c.repo.Create(ctx, v); e != nil {
		response.Error(ctx, http.StatusConflict, "budget already exists")
		return
	}
	response.Created(ctx, v)
}
func (c *Controller) Update(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	v, ok := c.bind(ctx, org, ctx.Param("budget_id"))
	if !ok {
		return
	}
	if e := c.repo.Update(ctx, org, ctx.Param("budget_id"), v); errors.Is(e, gorm.ErrRecordNotFound) {
		response.NotFound(ctx, "budget not found")
		return
	} else if e != nil {
		response.InternalError(ctx, "failed to update budget")
		return
	}
	response.OK(ctx, v)
}
func (c *Controller) Delete(ctx *gin.Context) {
	org, ok := c.allowed(ctx, true)
	if !ok {
		return
	}
	e := c.repo.Delete(ctx, org, ctx.Param("budget_id"))
	if errors.Is(e, gorm.ErrRecordNotFound) {
		response.NotFound(ctx, "budget not found")
		return
	}
	if e != nil {
		response.InternalError(ctx, "failed to delete budget")
		return
	}
	response.OKMessage(ctx, "budget deleted")
}
