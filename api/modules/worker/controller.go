package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Memberships interface {
	Membership(context.Context, string, string) (*organization.Membership, error)
}
type Controller struct {
	executor      *Executor
	organizations Memberships
	audits        AuditRepository
	now           func() time.Time
}

func NewController(e *Executor, o Memberships, audits ...AuditRepository) *Controller {
	var a AuditRepository
	if len(audits) > 0 {
		a = audits[0]
	}
	return &Controller{executor: e, organizations: o, audits: a, now: time.Now}
}

type request struct{ Wave, Branch, Brief, Worktree, Script string }
type result struct{ ID, State string }

func (c *Controller) access(ctx *gin.Context, actions ...string) (string, bool) {
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
		action := "unknown"
		if len(actions) > 0 {
			action = actions[0]
		}
		if !c.record(ctx, org, action, "", "", "role_rejected") {
			return "", false
		}
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
	org, ok := c.access(ctx, action)
	if !ok {
		return
	}
	var req request
	if ctx.ShouldBindJSON(&req) != nil {
		if !c.record(ctx, org, action, "", "", "validation_rejected") {
			return
		}
		response.BadRequest(ctx, "invalid request")
		return
	}
	j, err := c.executor.Enqueue(Request{OrganizationID: org, Wave: req.Wave, Branch: req.Branch, Brief: req.Brief, Worktree: req.Worktree, Script: req.Script})
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			if !c.record(ctx, org, action, req.Wave, "", "queue_rejected") {
				return
			}
			response.JSON(ctx, http.StatusConflict, map[string]any{"status": http.StatusConflict, "message": err.Error()})
			return
		}
		if !c.record(ctx, org, action, req.Wave, "", "rejected") {
			return
		}
		response.BadRequest(ctx, err.Error())
		return
	}
	if !c.record(ctx, org, action, req.Wave, j.ID, "accepted") {
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
	org, ok := c.access(ctx, "stop")
	if !ok {
		return
	}
	if err := c.executor.StopForOrganization(ctx.Param("worker_id"), org); err != nil {
		if !c.record(ctx, org, "stop", "", ctx.Param("worker_id"), "rejected") {
			return
		}
		if errors.Is(err, ErrNotFound) {
			response.NotFound(ctx, "worker not found")
		} else {
			response.BadRequest(ctx, err.Error())
		}
		return
	}
	if !c.record(ctx, org, "stop", "", ctx.Param("worker_id"), "accepted") {
		return
	}
	response.OKMessage(ctx, "worker stop requested")
}

func (c *Controller) record(ctx *gin.Context, org, action, wave, job, outcome string) bool {
	if c.audits == nil {
		return true
	}
	actor, _ := ctx.Get("account_id")
	now := c.now
	if now == nil {
		now = time.Now
	}
	when := now()
	if err := c.audits.Create(ctx, &ActionAudit{ID: store.NewUUID(), OrganizationID: org, ActorID: fmt.Sprint(actor), OccurredAt: when, Action: action, Wave: wave, JobID: job, Outcome: outcome, CreatedAt: when}); err != nil {
		response.InternalError(ctx, "failed to record worker action")
		return false
	}
	return true
}

// @Summary Get worker job
// @Produce json
// @Param worker_id path string true "Worker job ID"
// @Success 200 {object} response.SuccessResponse
// @Failure 401,403,404 {object} response.ErrorResponse
// @Router /api/v1/workers/{worker_id} [get]
func (c *Controller) Get(ctx *gin.Context) {
	org, ok := c.access(ctx, "get")
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
