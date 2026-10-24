package board

import (
	"net/http"

	"github.com/0Hoag/agent-ops/api/pkg/pagination"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Controller struct {
	repository *Repository
	hub        *Hub
}

func NewController(repository *Repository) *Controller {
	return NewControllerWithHub(repository, NewHub())
}
func NewControllerWithHub(repository *Repository, hub *Hub) *Controller {
	return &Controller{repository: repository, hub: hub}
}

// ListRepositories returns public repository selector data.
// @Summary List repositories
// @Produce json
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Success 200 {object} ListResponse
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/repositories [get]
func (c *Controller) ListRepositories(ctx *gin.Context) {
	p, ok := parsePage(ctx)
	if !ok {
		return
	}
	items, meta, err := c.repository.ListRepositories(ctx.Request.Context(), p.Page, p.PerPage)
	if err != nil {
		response.Error(ctx, 500, "failed to list repositories")
		return
	}
	response.JSON(ctx, http.StatusOK, ListResponse{Data: items, Meta: meta})
}

// ListBoard returns issues with linked pull requests and pipeline runs.
// @Summary List board issues
// @Produce json
// @Param repository_id query string true "Repository UUID"
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Success 200 {object} ListResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Router /api/v1/board [get]
func (c *Controller) ListBoard(ctx *gin.Context) {
	id := ctx.Query("repository_id")
	if _, err := uuid.Parse(id); err != nil {
		response.BadRequest(ctx, "invalid repository_id")
		return
	}
	p, ok := parsePage(ctx)
	if !ok {
		return
	}
	exists, err := c.repository.RepositoryExists(ctx.Request.Context(), id)
	if err != nil {
		response.Error(ctx, 500, "failed to find repository")
		return
	}
	if !exists {
		response.NotFound(ctx, "repository not found")
		return
	}
	items, meta, err := c.repository.ListIssues(ctx.Request.Context(), id, p.Page, p.PerPage)
	if err != nil {
		response.Error(ctx, 500, "failed to list board")
		return
	}
	response.JSON(ctx, http.StatusOK, ListResponse{Data: items, Meta: meta})
}

func parsePage(ctx *gin.Context) (pagination.Params, bool) {
	p := pagination.ParseWithMax(ctx, 100)
	if ctx.Query("page") != "" && (p.Page == 1 && ctx.Query("page") != "1") {
		response.BadRequest(ctx, "invalid page")
		return p, false
	}
	if ctx.Query("per_page") != "" && p.PerPage == pagination.DefaultPerPage && ctx.Query("per_page") != "50" {
		response.BadRequest(ctx, "invalid per_page")
		return p, false
	}
	return p, true
}
