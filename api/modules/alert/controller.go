package alert

import (
	"net/http"

	"github.com/0Hoag/agent-ops/api/pkg/middleware"
	"github.com/0Hoag/agent-ops/api/pkg/pagination"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Controller struct{ repository *Repository }

type HistoryResponse struct {
	Data []Occurrence                 `json:"data"`
	Meta pagination.PaginatorResponse `json:"meta"`
}

func NewController(repository *Repository) *Controller { return &Controller{repository: repository} }

// History lists persisted alert occurrences for the authenticated organization.
// @Summary List alert history
// @Produce json
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Success 200 {object} HistoryResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/alerts/history [get]
func (c *Controller) History(ctx *gin.Context) {
	org, ok := middleware.OrganizationID(ctx)
	if !ok {
		response.Unauthorized(ctx, "organization selection required")
		return
	}
	p := pagination.Parse(ctx)
	rows, total, err := c.repository.List(ctx.Request.Context(), org, p.Page, p.PerPage)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to list alert history")
		return
	}
	response.JSON(ctx, http.StatusOK, HistoryResponse{Data: rows, Meta: p.ToResponse(total)})
}
