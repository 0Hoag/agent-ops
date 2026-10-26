package organization

import (
	"errors"
	"net/http"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

func (c *Controller) result(ctx *gin.Context, err error) {
	if err == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	if errors.Is(err, ErrNotMember) {
		response.NotFound(ctx, "organization not found")
		return
	}
	if errors.Is(err, ErrForbidden) {
		response.Forbidden(ctx, "insufficient organization permissions")
		return
	}
	if errors.Is(err, ErrLastOwner) {
		response.Conflict(ctx, "cannot remove or demote the last owner")
		return
	}
	response.Conflict(ctx, "membership conflict")
}
