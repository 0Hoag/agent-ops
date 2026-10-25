package insight

import (
	"errors"
	"net/http"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Controller struct{ repository *Repository }

func NewController(repository *Repository) *Controller { return &Controller{repository: repository} }

// Get returns computed repository delivery insights.
// @Summary Get repository insights
// @Produce json
// @Param repository_id query string true "Repository UUID" format(uuid)
// @Success 200 {object} Envelope
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/insights [get]
func (c *Controller) Get(ctx *gin.Context) {
	id := ctx.Query("repository_id")
	if _, err := uuid.Parse(id); err != nil {
		response.BadRequest(ctx, "invalid repository_id")
		return
	}
	data, err := c.repository.Build(ctx.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(ctx, "repository not found")
		return
	}
	if err != nil {
		response.Error(ctx, 500, "failed to build insights")
		return
	}
	response.JSON(ctx, http.StatusOK, gin.H{"data": data})
}
