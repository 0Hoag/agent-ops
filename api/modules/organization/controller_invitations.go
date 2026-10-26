package organization

import (
	"errors"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

// CreateInvitation creates a local invitation link. The token is returned once and only its hash is persisted.
// @Summary Create organization invitation
// @Accept json
// @Produce json
// @Param organization_id path string true "Organization UUID"
// @Param request body CreateInvitationRequest true "Invitation"
// @Success 201 {object} response.SuccessResponse
// @Failure 400,401,403,404 {object} response.ErrorResponse
// @Router /api/v1/organizations/{organization_id}/invitations [post]
func (c *Controller) CreateInvitation(ctx *gin.Context) {
	actor, ok := c.accountID(ctx)
	if !ok {
		return
	}
	var req CreateInvitationRequest
	if ctx.ShouldBindJSON(&req) != nil || req.Role == RoleOwner || !validRole(req.Role) {
		response.BadRequest(ctx, "invalid invitation role")
		return
	}
	seconds := req.ExpiresInSeconds
	if seconds == 0 {
		seconds = 7 * 24 * 60 * 60
	}
	if seconds < 1 || seconds > 30*24*60*60 {
		response.BadRequest(ctx, "invalid invitation expiry")
		return
	}
	invite, token, err := c.repository.CreateInvitation(ctx.Request.Context(), ctx.Param("organization_id"), actor, req.Role, time.Now().Add(time.Duration(seconds)*time.Second))
	if errors.Is(err, ErrNotMember) {
		response.NotFound(ctx, "organization not found")
		return
	}
	if errors.Is(err, ErrForbidden) {
		response.Forbidden(ctx, "insufficient organization permissions")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to create invitation")
		return
	}
	response.Created(ctx, CreateInvitationResponse{Invitation: *invite, Token: token, Link: "/invite/" + token})
}

// AcceptInvitation accepts an invitation for the authenticated account.
// @Summary Accept organization invitation
// @Accept json
// @Produce json
// @Param request body AcceptInvitationRequest true "Invitation token"
// @Success 201 {object} response.SuccessResponse
// @Failure 400,401,409 {object} response.ErrorResponse
// @Router /api/v1/organization-invitations/accept [post]
func (c *Controller) AcceptInvitation(ctx *gin.Context) {
	account, ok := c.accountID(ctx)
	if !ok {
		return
	}
	var req AcceptInvitationRequest
	if ctx.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Token) == "" {
		response.BadRequest(ctx, "invalid invitation token")
		return
	}
	membership, err := c.repository.AcceptInvitation(ctx.Request.Context(), account, req.Token)
	if errors.Is(err, ErrInvitationInvalid) {
		response.Conflict(ctx, "invitation is invalid, expired, or already accepted")
		return
	}
	if err != nil {
		response.InternalError(ctx, "failed to accept invitation")
		return
	}
	response.Created(ctx, membership)
}
