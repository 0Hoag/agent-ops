package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RunRepository interface {
	Create(context.Context, *AgentRun) error
	IsConfiguredRepository(context.Context, string, []string) (bool, error)
	List(context.Context, Filter) ([]AgentRun, error)
	Aggregate(context.Context, Filter, map[string]Price) ([]Aggregate, error)
}

// Controller exposes agent run persistence and reporting endpoints.
// @Tags agent-runs
type Controller struct {
	repository RunRepository
	token      string
	prices     map[string]Price
	configured []string
}

func NewController(repository RunRepository, token string, prices map[string]Price) *Controller {
	return &Controller{repository: repository, token: token, prices: prices}
}

func NewControllerWithConfiguredRepositories(repository RunRepository, token string, prices map[string]Price, configured []string) *Controller {
	return &Controller{repository: repository, token: token, prices: prices, configured: configured}
}

// Create persists a validated normalized usage record.
// @Summary Create an agent run
// @Accept json
// @Produce json
// @Param X-Agent-Run-Token header string true "Static agent run token"
// @Param request body CreateInput true "Agent run"
// @Success 201 {object} CreatedResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Security AgentRunToken
// @Router /api/v1/agent-runs [post]
func (c *Controller) Create(ctx *gin.Context) {
	if c.token == "" || ctx.GetHeader("X-Agent-Run-Token") != c.token {
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
	if c.configured != nil {
		configured, err := c.repository.IsConfiguredRepository(ctx.Request.Context(), in.RepositoryID, c.configured)
		if err != nil {
			response.Error(ctx, http.StatusInternalServerError, "failed to validate repository")
			return
		}
		if !configured {
			response.Error(ctx, http.StatusUnauthorized, "repository is not configured for agent ingestion")
			return
		}
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

// List returns runs filtered by repository, wave, and issue number.
// @Summary List agent runs
// @Produce json
// @Param repository_id query string false "Repository UUID"
// @Param wave query string false "Wave"
// @Param issue_number query int false "Issue number"
// @Success 200 {object} ListResponse
// @Router /api/v1/agent-runs [get]
func (c *Controller) List(ctx *gin.Context) {
	filter, err := requestFilter(ctx)
	if err != nil {
		response.BadRequest(ctx, "invalid issue_number")
		return
	}
	items, err := c.repository.List(ctx.Request.Context(), filter)
	if err != nil {
		response.Error(ctx, 500, "failed to list agent runs")
		return
	}
	response.OK(ctx, items)
}

// Aggregate returns usage and cost grouped by wave, worker, and issue.
// @Summary Aggregate agent runs
// @Produce json
// @Param repository_id query string false "Repository UUID"
// @Param wave query string false "Wave"
// @Param issue_number query int false "Issue number"
// @Success 200 {object} AggregateResponse
// @Router /api/v1/agent-runs/aggregate [get]
func (c *Controller) Aggregate(ctx *gin.Context) {
	filter, err := requestFilter(ctx)
	if err != nil {
		response.BadRequest(ctx, "invalid issue_number")
		return
	}
	items, err := c.repository.Aggregate(ctx.Request.Context(), filter, c.prices)
	if err != nil {
		response.Error(ctx, 500, "failed to aggregate agent runs")
		return
	}
	response.OK(ctx, items)
}

func requestFilter(ctx *gin.Context) (Filter, error) {
	filter := Filter{RepositoryID: ctx.Query("repository_id"), Wave: ctx.Query("wave")}
	if raw := ctx.Query("issue_number"); raw != "" {
		issue, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || issue < 1 {
			return Filter{}, fmt.Errorf("invalid issue number")
		}
		filter.IssueNumber = issue
	}
	return filter, nil
}
