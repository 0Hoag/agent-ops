package agentrun

import (
	"context"
	"encoding/json"
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
	var run AgentRun
	q := r.db.WithContext(ctx).Table("agent_runs ar").Joins("JOIN repositories repo ON repo.id=ar.repository_id").Where("ar.id = ? AND repo.organization_id = ?", id, organizationFromContext(ctx))
	if err := q.First(&run).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Table("agent_run_steps s").Where("s.run_id = ?", id).Order("s.sequence ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	price, err := r.priceAt(ctx, organizationFromContext(ctx), run.Model, run.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("resolve historical timeline price: %w", err)
	}
	if price != nil {
		var cumulative float64
		for i := range out {
			cumulative += stepCost(out[i], *price)
			value := cumulative
			out[i].CumulativeCostUSD = &value
		}
	}
	return out, nil
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
		seen := make(map[string]int, len(steps))
		for i := range steps {
			fingerprint := timelineStepFingerprint(steps[i])
			occurrence := seen[fingerprint]
			seen[fingerprint] = occurrence + 1
			steps[i].ID = uuid.NewMD5(uuid.Nil, []byte(id+":"+fingerprint+":"+strconv.Itoa(occurrence))).String()
			steps[i].RunID = id
			steps[i].Sequence = i
		}
		return tx.Table("agent_run_steps").Create(&steps).Error
	})
}

func timelineStepFingerprint(step RunStep) string {
	step.ID, step.RunID, step.Sequence, step.CumulativeCostUSD = "", "", 0, nil
	b, _ := json.Marshal(step)
	return string(b)
}

func stepCost(step RunStep, price Price) float64 {
	return float64(step.InputTokens)*price.InputPerMillion/1e6 +
		float64(step.CachedInputTokens)*price.CachedInputPerMillion/1e6 +
		float64(step.CacheWriteTokens)*price.CacheWritePerMillion/1e6 +
		float64(step.OutputTokens)*price.OutputPerMillion/1e6 +
		float64(step.ReasoningTokens)*price.ReasoningPerMillion/1e6
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
