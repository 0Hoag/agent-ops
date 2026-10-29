package wave

import (
	"errors"
	"net/http"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/pkg/pagination"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Controller struct {
	repository    *Repository
	organizations *organization.Repository
}

func NewController(r *Repository, o *organization.Repository) *Controller {
	return &Controller{repository: r, organizations: o}
}

type request struct {
	Name  string `json:"name"`
	Brief string `json:"brief"`
	State string `json:"state"`
}
type listResponse struct {
	Data []Wave                       `json:"data"`
	Meta pagination.PaginatorResponse `json:"meta"`
}

func (c *Controller) access(ctx *gin.Context, write bool) (string, bool) {
	v, ok := ctx.Get("organization_id")
	org, valid := v.(string)
	if !ok || !valid || org == "" {
		response.Unauthorized(ctx, "organization required")
		return "", false
	}
	a, _ := ctx.Get("account_id")
	account, _ := a.(string)
	member, err := c.organizations.Membership(ctx, org, account)
	if err != nil {
		response.Forbidden(ctx, "organization access denied")
		return "", false
	}
	if write && member.Role != organization.RoleOwner && member.Role != organization.RoleAdmin {
		response.Forbidden(ctx, "wave write requires owner or admin")
		return "", false
	}
	return org, true
}
func (c *Controller) List(ctx *gin.Context) {
	org, ok := c.access(ctx, false)
	if !ok {
		return
	}
	p := pagination.Parse(ctx)
	rows, total, err := c.repository.List(ctx, org, p.Page, p.PerPage)
	if err != nil {
		response.InternalError(ctx, "failed to list waves")
		return
	}
	response.JSON(ctx, http.StatusOK, listResponse{rows, p.ToResponse(total)})
}
func (c *Controller) Get(ctx *gin.Context) {
	org, ok := c.access(ctx, false)
	if !ok {
		return
	}
	row, err := c.repository.Get(ctx, org, ctx.Param("wave_id"))
	if errors.Is(err, ErrNotFound) {
		response.NotFound(ctx, "wave not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to get wave")
		return
	}
	response.OK(ctx, row)
}
func (c *Controller) Create(ctx *gin.Context) {
	org, ok := c.access(ctx, true)
	if !ok {
		return
	}
	var req request
	if ctx.ShouldBindJSON(&req) != nil {
		response.BadRequest(ctx, "invalid request")
		return
	}
	row, err := c.repository.Create(ctx, org, req.Name, req.Brief)
	if err != nil {
		response.BadRequest(ctx, err.Error())
		return
	}
	response.Created(ctx, row)
}
func (c *Controller) Update(ctx *gin.Context) {
	org, ok := c.access(ctx, true)
	if !ok {
		return
	}
	var req request
	if ctx.ShouldBindJSON(&req) != nil {
		response.BadRequest(ctx, "invalid request")
		return
	}
	row, err := c.repository.Update(ctx, org, ctx.Param("wave_id"), req.Name, req.Brief, req.State)
	if errors.Is(err, ErrNotFound) {
		response.NotFound(ctx, "wave not found")
		return
	}
	if errors.Is(err, ErrInvalidTransition) {
		response.BadRequest(ctx, err.Error())
		return
	}
	if err != nil {
		response.BadRequest(ctx, err.Error())
		return
	}
	response.OK(ctx, row)
}
func (c *Controller) Delete(ctx *gin.Context) {
	org, ok := c.access(ctx, true)
	if !ok {
		return
	}
	err := c.repository.Delete(ctx, org, ctx.Param("wave_id"))
	if errors.Is(err, ErrNotFound) {
		response.NotFound(ctx, "wave not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to delete wave")
		return
	}
	ctx.Status(http.StatusNoContent)
}
