package worker

import (
	"context"
	"errors"
	"net/http"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Memberships interface {
	Membership(context.Context, string, string) (*organization.Membership, error)
}
type Controller struct {
	executor      *Executor
	organizations Memberships
}

func NewController(e *Executor, o Memberships) *Controller {
	return &Controller{executor: e, organizations: o}
}

type request struct{ Wave, Branch, Brief, Worktree, Script string }
type result struct{ ID, State string }

func (c *Controller) access(ctx *gin.Context) (string, bool) {
	v, ok := ctx.Get("organization_id")
	org, valid := v.(string)
	if !ok || !valid || org == "" {
		response.Unauthorized(ctx, "organization required")
		return "", false
	}
	a, _ := ctx.Get("account_id")
	account, _ := a.(string)
	m, err := c.organizations.Membership(ctx, org, account)
	if err != nil {
		response.Forbidden(ctx, "organization access denied")
		return "", false
	}
	if m.Role != organization.RoleOwner && m.Role != organization.RoleAdmin {
		response.Forbidden(ctx, "worker lifecycle requires owner or admin")
		return "", false
	}
	return org, true
}

// @Summary Start worker
// @Accept json
// @Produce json
// @Param request body request true "Worker request"
// @Success 201 {object} response.SuccessResponse
// @Router /api/v1/workers/start [post]
func (c *Controller) Start(ctx *gin.Context) { c.submit(ctx, "start") }

// @Summary Restart worker
// @Accept json
// @Produce json
// @Param request body request true "Worker request"
// @Success 201 {object} response.SuccessResponse
// @Router /api/v1/workers/restart [post]
func (c *Controller) Restart(ctx *gin.Context) { c.submit(ctx, "restart") }
func (c *Controller) submit(ctx *gin.Context, action string) {
	org, ok := c.access(ctx)
	if !ok {
		return
	}
	var req request
	if ctx.ShouldBindJSON(&req) != nil {
		response.BadRequest(ctx, "invalid request")
		return
	}
	j, err := c.executor.Enqueue(Request{OrganizationID: org, Wave: req.Wave, Branch: req.Branch, Brief: req.Brief, Worktree: req.Worktree, Script: req.Script})
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			response.JSON(ctx, http.StatusConflict, map[string]any{"status": http.StatusConflict, "message": err.Error()})
			return
		}
		response.BadRequest(ctx, err.Error())
		return
	}
	response.Created(ctx, result{j.ID, j.State})
}

// @Summary Stop worker
// @Produce json
// @Param worker_id path string true "Worker job ID"
// @Success 200 {object} response.MessageResponse
// @Router /api/v1/workers/{worker_id}/stop [post]
func (c *Controller) Stop(ctx *gin.Context) {
	org, ok := c.access(ctx)
	if !ok {
		return
	}
	if err := c.executor.StopForOrganization(ctx.Param("worker_id"), org); err != nil {
		if errors.Is(err, ErrNotFound) {
			response.NotFound(ctx, "worker not found")
		} else {
			response.BadRequest(ctx, err.Error())
		}
		return
	}
	response.OKMessage(ctx, "worker stop requested")
}

// @Summary Get worker job
// @Produce json
// @Param worker_id path string true "Worker job ID"
// @Success 200 {object} response.SuccessResponse
// @Failure 401,403,404 {object} response.ErrorResponse
// @Router /api/v1/workers/{worker_id} [get]
func (c *Controller) Get(ctx *gin.Context) {
	org, ok := c.access(ctx)
	if !ok {
		return
	}
	j, err := c.executor.GetForOrganization(ctx.Param("worker_id"), org)
	if errors.Is(err, ErrNotFound) {
		response.NotFound(ctx, "worker not found")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to get worker")
		return
	}
	response.OK(ctx, j)
}
