package work

import (
	"errors"
	"strconv"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Controller struct{ r *Repository }

func NewController(r *Repository) *Controller { return &Controller{r: r} }

// List returns pull requests and issues for a repository.
// @Summary List repository work
// @Produce json
// @Param repository_id query string true "Repository UUID"
// @Param state query string false "State"
// @Param q query string false "Title search"
// @Param page query int false "Page"
// @Success 200 {object} ListResponse
// @Router /api/v1/work [get]
func (c *Controller) List(x *gin.Context) {
	id := x.Query("repository_id")
	p, _ := strconv.Atoi(x.DefaultQuery("page", "1"))
	if p < 1 {
		response.BadRequest(x, "invalid page")
		return
	}
	items, total, e := c.r.List(x.Request.Context(), id, x.Query("state"), x.Query("q"), p, 50)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		response.NotFound(x, "repository not found")
		return
	}
	if e != nil {
		response.Error(x, 500, "failed to list work")
		return
	}
	response.JSON(x, 200, ListResponse{Data: items, Meta: Meta{Page: p, PerPage: 50, Total: total}})
}

// Detail returns a work item and its linked agent runs.
// @Summary Get work detail
// @Produce json
// @Param repository_id query string true "Repository UUID"
// @Param kind path string true "pr or issue"
// @Param number path int true "Work number"
// @Success 200 {object} Detail
// @Router /api/v1/work/{kind}/{number} [get]
func (c *Controller) Detail(x *gin.Context) {
	n, e := strconv.ParseInt(x.Param("number"), 10, 64)
	if e != nil {
		response.BadRequest(x, "invalid number")
		return
	}
	d, e := c.r.Detail(x.Request.Context(), x.Query("repository_id"), x.Param("kind"), n)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		response.NotFound(x, "work not found")
		return
	}
	if e != nil {
		response.Error(x, 500, "failed to get work")
		return
	}
	response.JSON(x, 200, d)
}
