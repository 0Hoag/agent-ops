package auth

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/account"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Controller struct{ service *Service }

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func NewController(service *Service) *Controller { return &Controller{service: service} }

// @Summary Login
// @Accept json
// @Produce json
// @Param credentials body LoginRequest true "Credentials"
// @Success 200 {object} LoginResponse
// @Failure 400,401 {object} response.ErrorResponse
// @Router /api/v1/auth/login [post]
func (controller *Controller) Login(c *gin.Context) {
	var request LoginRequest
	if c.ShouldBindJSON(&request) != nil || !emailPattern.MatchString(strings.TrimSpace(request.Email)) || request.Password == "" {
		response.BadRequest(c, "invalid request")
		return
	}
	if len(request.Password) > 72 {
		response.BadRequest(c, "invalid request")
		return
	}
	token, err := controller.service.Login(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		if errors.Is(err, account.ErrInvalidCredentials) {
			response.Unauthorized(c, "invalid credentials")
			return
		}
		response.InternalError(c, "authentication failed")
		return
	}
	c.JSON(http.StatusOK, LoginResponse{Token: token})
}

// @Summary Current session
// @Produce json
// @Success 200 {object} SessionResponse
// @Failure 401 {object} response.ErrorResponse
// @Router /api/v1/auth/session [get]
func (controller *Controller) Session(c *gin.Context) {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		response.Unauthorized(c, "invalid token")
		return
	}
	session, err := controller.service.Session(parts[1])
	if err != nil {
		response.Unauthorized(c, "invalid token")
		return
	}
	c.JSON(http.StatusOK, session)
}

// @Summary Select organization
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer account token"
// @Param request body SelectOrganizationRequest true "Organization"
// @Success 200 {object} LoginResponse
// @Failure 400,401,404 {object} response.ErrorResponse
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
