package agentrun

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TimelineRepository interface {
	Steps(context.Context, string) ([]RunStep, error)
	ReplaceSteps(context.Context, string, []RunStep) error
}
type TimelineInput struct {
	Steps []RunStep `json:"steps"`
}

func (r *Repository) Steps(ctx context.Context, id string) ([]RunStep, error) {
	var out []RunStep
	err := r.db.WithContext(ctx).Table("agent_run_steps s").Select("s.*").Joins("JOIN agent_runs ar ON ar.id=s.run_id JOIN repositories repo ON repo.id=ar.repository_id").Where("s.run_id = ? AND repo.organization_id = ?", id, organizationFromContext(ctx)).Order("s.sequence ASC").Find(&out).Error
	return out, err
}
func (r *Repository) ReplaceSteps(ctx context.Context, id string, steps []RunStep) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run AgentRun
		query := tx.Table("agent_runs ar").Joins("JOIN repositories repo ON repo.id = ar.repository_id").Where("ar.id = ?", id)
		if org := organizationFromContext(ctx); org != "" {
			query = query.Where("repo.organization_id = ?", org)
		}
		if err := query.First(&run).Error; err != nil {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Exec("DELETE FROM agent_run_steps WHERE run_id = ?", id).Error; err != nil {
			return err
		}
		for i := range steps {
			steps[i].ID = uuid.NewMD5(uuid.Nil, []byte(id+":"+strconv.Itoa(i))).String()
			steps[i].RunID = id
			steps[i].Sequence = i
		}
		return tx.Table("agent_run_steps").Create(&steps).Error
	})
}

// Timeline returns sanitized ordered steps for an organization-scoped run.
// @Summary Get run timeline
// @Produce json
// @Param run_id path string true "Run UUID"
// @Success 200 {object} response.SuccessResponse
// @Security BearerAuth
// @Router /api/v1/agent-runs/{run_id}/timeline [get]
func (c *Controller) Timeline(ctx *gin.Context) {
	org, ok := ctx.Get("organization_id")
	if !ok || org == "" {
		response.Unauthorized(ctx, "organization required")
		return
	}
	repo, ok := c.repository.(TimelineRepository)
	if !ok {
		response.Error(ctx, 500, "timeline unavailable")
		return
	}
	id := ctx.Param("run_id")
	steps, err := repo.Steps(ctx.Request.Context(), id)
	if err != nil {
		response.NotFound(ctx, "run not found")
		return
	}
	response.OK(ctx, steps)
}

// IngestTimeline replaces sanitized steps idempotently for a run.
// @Summary Ingest run timeline
// @Accept json
// @Produce json
// @Param run_id path string true "Run UUID"
// @Param request body TimelineInput true "Sanitized steps"
// @Success 200 {object} response.SuccessResponse
// @Security AgentRunToken
// @Router /api/v1/agent-runs/{run_id}/timeline [post]
func (c *Controller) IngestTimeline(ctx *gin.Context) {
	if c.token == "" || ctx.GetHeader("X-Agent-Run-Token") != c.token {
		response.Error(ctx, http.StatusUnauthorized, "unauthorized")
		return
	}
	repo, ok := c.repository.(TimelineRepository)
	if !ok {
		response.Error(ctx, 500, "timeline unavailable")
		return
	}
	var in TimelineInput
	if ctx.ShouldBindJSON(&in) != nil || len(in.Steps) > 10000 {
		response.BadRequest(ctx, "invalid steps")
		return
	}
	if err := repo.ReplaceSteps(ctx, ctx.Param("run_id"), in.Steps); err != nil {
		response.Error(ctx, 500, fmt.Sprintf("failed to store timeline: %v", err))
		return
	}
	response.OK(ctx, in.Steps)
}
