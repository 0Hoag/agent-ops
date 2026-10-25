package organization

import (
	"errors"
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Controller struct {
	repository *Repository
	sessions   *auth.Service
}

func NewController(repository *Repository, sessions *auth.Service) *Controller {
	return &Controller{repository: repository, sessions: sessions}
}

func (c *Controller) accountID(ctx *gin.Context) (string, bool) {
	p := strings.Fields(ctx.GetHeader("Authorization"))
	if len(p) != 2 || !strings.EqualFold(p[0], "Bearer") {
		response.Unauthorized(ctx, "invalid token")
		return "", false
	}
	s, err := c.sessions.Session(p[1])
	if err != nil {
		response.Unauthorized(ctx, "invalid token")
		return "", false
	}
	return s.AccountID, true
}

// @Summary Create organization
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Organization"
// @Success 201 {object} response.SuccessResponse
// @Failure 400,401 {object} response.ErrorResponse
// @Router /api/v1/organizations [post]
func (c *Controller) Create(ctx *gin.Context) {
	id, ok := c.accountID(ctx)
	if !ok {
		return
	}
	var req CreateRequest
	if ctx.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		response.BadRequest(ctx, "invalid request")
		return
	}
	o, err := c.repository.Create(ctx, id, req.Name)
	if err != nil {
		response.InternalError(ctx, "failed to create organization")
		return
	}
	response.Created(ctx, o)
}

// @Summary List organizations
// @Produce json
// @Success 200 {object} response.SuccessResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/organizations [get]
func (c *Controller) List(ctx *gin.Context) {
	id, ok := c.accountID(ctx)
	if !ok {
		return
	}
	v, err := c.repository.List(ctx, id)
	if err != nil {
		response.InternalError(ctx, "failed to list organizations")
		return
	}
	response.OK(ctx, v)
}

// @Summary Organization detail
// @Produce json
// @Param organization_id path string true "Organization UUID"
// @Success 200 {object} response.SuccessResponse
// @Failure 401,404 {object} response.ErrorResponse
// @Router /api/v1/organizations/{organization_id} [get]
func (c *Controller) Detail(ctx *gin.Context) {
	id, ok := c.accountID(ctx)
	if !ok {
		return
	}
	v, err := c.repository.Detail(ctx, ctx.Param("organization_id"), id)
	if errors.Is(err, ErrNotMember) {
		response.NotFound(ctx, "organization not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to find organization")
		return
	}
	response.OK(ctx, v)
}

// @Summary List organization members
// @Produce json
// @Param organization_id path string true "Organization UUID"
// @Success 200 {object} response.SuccessResponse
// @Failure 401,404 {object} response.ErrorResponse
// @Router /api/v1/organizations/{organization_id}/members [get]
func (c *Controller) Members(ctx *gin.Context) {
	id, ok := c.accountID(ctx)
	if !ok {
		return
	}
	v, err := c.repository.Members(ctx, ctx.Param("organization_id"), id)
	if errors.Is(err, ErrNotMember) {
		response.NotFound(ctx, "organization not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to list members")
		return
	}
	response.OK(ctx, v)
}

// @Summary Add organization member
// @Accept json
// @Produce json
// @Param organization_id path string true "Organization UUID"
// @Param request body AddMemberRequest true "Membership"
// @Success 204 {object} response.MessageResponse
// @Failure 400,401,403,404,409 {object} response.ErrorResponse
// @Router /api/v1/organizations/{organization_id}/members [post]
func (c *Controller) Add(ctx *gin.Context) { c.change(ctx, "add") }

// @Summary Update member role
// @Accept json
// @Produce json
// @Param organization_id path string true "Organization UUID"
// @Param account_id path string true "Account UUID"
// @Param request body UpdateRoleRequest true "Role"
// @Success 204 {object} response.MessageResponse
// @Failure 400,401,403,404,409 {object} response.ErrorResponse
// @Router /api/v1/organizations/{organization_id}/members/{account_id} [patch]
func (c *Controller) Role(ctx *gin.Context) { c.change(ctx, "role") }

// @Summary Remove organization member
// @Produce json
// @Param organization_id path string true "Organization UUID"
// @Param account_id path string true "Account UUID"
// @Success 204 {object} response.MessageResponse
// @Failure 401,403,404,409 {object} response.ErrorResponse
// @Router /api/v1/organizations/{organization_id}/members/{account_id} [delete]
func (c *Controller) Remove(ctx *gin.Context) {
	id, ok := c.accountID(ctx)
	if !ok {
		return
	}
	err := c.repository.Remove(ctx, ctx.Param("organization_id"), id, ctx.Param("account_id"))
	c.result(ctx, err)
}
func (c *Controller) change(ctx *gin.Context, action string) {
	id, ok := c.accountID(ctx)
	if !ok {
		return
	}
	var req AddMemberRequest
	if action == "role" {
		var r UpdateRoleRequest
		if ctx.ShouldBindJSON(&r) != nil {
			response.BadRequest(ctx, "invalid request")
			return
		}
		req.Role = r.Role
	} else if ctx.ShouldBindJSON(&req) != nil {
		response.BadRequest(ctx, "invalid request")
		return
	}
	var err error
	if action == "add" {
		err = c.repository.Add(ctx, ctx.Param("organization_id"), id, req.AccountID, req.Role)
	} else {
		err = c.repository.UpdateRole(ctx, ctx.Param("organization_id"), id, ctx.Param("account_id"), req.Role)
	}
	c.result(ctx, err)
}
