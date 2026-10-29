package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

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
type HistoricalAggregator interface {
	AggregateHistorical(context.Context, Filter, map[string]Price) ([]Aggregate, error)
}

// Controller exposes agent run persistence and reporting endpoints.
// @Tags agent-runs
type Controller struct {
	repository      RunRepository
	token           string
	prices          map[string]Price
	configured      []string
	exchangeRateVND float64
}

func (c *Controller) SetExchangeRateVND(rate float64) {
	if rate > 0 {
		c.exchangeRateVND = rate
	}
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
	if in.Harness != "" || in.SessionID != "" {
		run.Harness, run.SessionID = &in.Harness, &in.SessionID
	}
	run.BillingType = in.BillingType
	if run.BillingType == "" {
		run.BillingType = "api"
	}
	if run.BillingType != "api" && run.BillingType != "subscription" {
		response.BadRequest(ctx, "billing_type must be api or subscription")
		return
	}
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
	page, perPage, err := parsePagination(ctx)
	if err != nil {
		response.BadRequest(ctx, err.Error())
		return
	}
	var items []AgentRun
	var total int64
	if paged, ok := c.repository.(interface {
		ListPage(context.Context, Filter, int, int) ([]AgentRun, int64, error)
	}); ok {
		items, total, err = paged.ListPage(ctx.Request.Context(), filter, page, perPage)
	} else {
		items, err = c.repository.List(ctx.Request.Context(), filter)
		total = int64(len(items))
	}
	if err != nil {
		response.Error(ctx, 500, "failed to list agent runs")
		return
	}
	response.JSON(ctx, http.StatusOK, ListResponse{Status: http.StatusOK, Data: items, Meta: map[string]any{"page": page, "per_page": perPage, "total": total}})
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
	var items []Aggregate
	var aggregateErr error
	if historical, ok := c.repository.(HistoricalAggregator); ok {
		items, aggregateErr = historical.AggregateHistorical(ctx.Request.Context(), filter, c.prices)
	} else {
		items, aggregateErr = c.repository.Aggregate(ctx.Request.Context(), filter, c.prices)
	}
	if aggregateErr != nil {
		response.Error(ctx, 500, "failed to aggregate agent runs")
		return
	}
	if c.exchangeRateVND > 0 {
		for i := range items {
			if items[i].CostUSD != nil {
				v := *items[i].CostUSD * c.exchangeRateVND
				items[i].CostVND = &v
			}
		}
	}
	response.OK(ctx, items)
}

func requestFilter(ctx *gin.Context) (Filter, error) {
	filter := Filter{RepositoryID: ctx.Query("repository_id"), Wave: ctx.Query("wave"), Harness: ctx.Query("harness"), Model: ctx.Query("model")}
	if raw := ctx.Query("pr_number"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 {
			return Filter{}, fmt.Errorf("invalid pr_number")
		}
		filter.PRNumber = n
	}
	if raw := ctx.Query("issue_number"); raw != "" {
		issue, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || issue < 1 {
			return Filter{}, fmt.Errorf("invalid issue number")
		}
		filter.IssueNumber = issue
	}
	for name, dst := range map[string]**time.Time{"started_after": &filter.StartedAfter, "started_before": &filter.StartedBefore} {
		if raw := ctx.Query(name); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return Filter{}, fmt.Errorf("invalid %s", name)
			}
			*dst = &t
		}
	}
	if filter.StartedAfter != nil && filter.StartedBefore != nil && filter.StartedAfter.After(*filter.StartedBefore) {
		return Filter{}, fmt.Errorf("started_after must not be after started_before")
	}
	return filter, nil
}

func parsePagination(ctx *gin.Context) (int, int, error) {
	page, per := 1, 50
	var err error
	if raw := ctx.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			return 0, 0, fmt.Errorf("invalid page")
		}
	}
	if raw := ctx.Query("per_page"); raw != "" {
		per, err = strconv.Atoi(raw)
		if err != nil || per < 1 || per > 100 {
			return 0, 0, fmt.Errorf("invalid per_page")
		}
	}
	return page, per, nil
}
