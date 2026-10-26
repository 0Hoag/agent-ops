package auth

import (
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

type SelectOrganizationRequest struct {
	OrganizationID string `json:"organization_id"`
}

// @Summary Select organization
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer account token"
// @Param request body SelectOrganizationRequest true "Organization"
// @Success 200 {object} LoginResponse
// @Failure 401,404 {object} response.ErrorResponse
// @Router /api/v1/auth/select-organization [post]

func (controller *Controller) SelectOrganization(c *gin.Context) {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		response.Unauthorized(c, "invalid token")
		return
	}
	var in SelectOrganizationRequest
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.OrganizationID) == "" {
		response.BadRequest(c, "invalid request")
		return
	}
	token, err := controller.service.SelectOrganization(c.Request.Context(), parts[1], in.OrganizationID)
	if err != nil {
		response.NotFound(c, "organization not found")
		return
	}
	c.JSON(http.StatusOK, LoginResponse{Token: token})
}
