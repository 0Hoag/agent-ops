package agentrun

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RunRepository interface {
	Create(context.Context, *AgentRun) error
	List(context.Context, Filter) ([]AgentRun, error)
	Aggregate(context.Context, Filter, map[string]Price) ([]Aggregate, error)
}

type Controller struct {
	repository RunRepository
	token      string
	prices     map[string]Price
}

func NewController(repository RunRepository, token string, prices map[string]Price) *Controller {
	return &Controller{repository: repository, token: token, prices: prices}
}
func (c *Controller) Create(ctx *gin.Context) {
	if c.token == "" || !strings.EqualFold(ctx.GetHeader("X-Agent-Run-Token"), c.token) {
		response.Error(ctx, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in CreateInput
	if ctx.ShouldBindJSON(&in) != nil {
		response.BadRequest(ctx, "invalid agent run")
		return
	}
	if _, err := uuid.Parse(in.RepositoryID); err != nil {
		response.BadRequest(ctx, "invalid repository_id")
		return
	}
	if in.InputTokens < 0 || in.CachedInputTokens < 0 || in.CacheWriteTokens < 0 || in.OutputTokens < 0 || in.ReasoningTokens < 0 {
		response.BadRequest(ctx, "token usage must be non-negative")
		return
	}
	if in.EndedAt.Before(in.StartedAt) {
		response.BadRequest(ctx, "ended_at must not precede started_at")
		return
	}
	b, err := json.Marshal(in.IssueNumbers)
	if err != nil {
		response.BadRequest(ctx, "invalid issue_numbers")
		return
	}
	run := AgentRun{ID: store.NewUUID(), RepositoryID: store.UUID(in.RepositoryID), Wave: in.Wave, Worker: in.Worker, Model: in.Model, IssueNumbers: b, PRNumber: in.PRNumber, InputTokens: in.InputTokens, CachedInputTokens: in.CachedInputTokens, CacheWriteTokens: in.CacheWriteTokens, OutputTokens: in.OutputTokens, ReasoningTokens: in.ReasoningTokens, StartedAt: in.StartedAt, EndedAt: in.EndedAt}
	if err := c.repository.Create(ctx.Request.Context(), &run); err != nil {
		response.Error(ctx, 500, "failed to create agent run")
		return
	}
	response.Created(ctx, run)
}
func (c *Controller) List(ctx *gin.Context) {
	items, err := c.repository.List(ctx.Request.Context(), requestFilter(ctx))
	if err != nil {
		response.Error(ctx, 500, "failed to list agent runs")
		return
	}
	response.OK(ctx, items)
}
func (c *Controller) Aggregate(ctx *gin.Context) {
	items, err := c.repository.Aggregate(ctx.Request.Context(), requestFilter(ctx), c.prices)
	if err != nil {
		response.Error(ctx, 500, "failed to aggregate agent runs")
		return
	}
	response.OK(ctx, items)
}

func requestFilter(ctx *gin.Context) Filter {
	issue, _ := strconv.ParseInt(ctx.Query("issue_number"), 10, 64)
	return Filter{RepositoryID: ctx.Query("repository_id"), Wave: ctx.Query("wave"), IssueNumber: issue}
}
