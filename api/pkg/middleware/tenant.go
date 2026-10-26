package middleware

import (
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/agentrun"
	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/modules/insight"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

func RequireOrganization(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := strings.Fields(c.GetHeader("Authorization"))
		if len(p) != 2 || !strings.EqualFold(p[0], "Bearer") {
			response.Unauthorized(c, "invalid token")
			c.Abort()
			return
		}
		s, err := service.Session(p[1])
		if err != nil || s.OrganizationID == "" {
			response.Unauthorized(c, "organization selection required")
			c.Abort()
			return
		}
		c.Set("organization_id", s.OrganizationID)
		c.Request = c.Request.WithContext(board.WithOrganization(c.Request.Context(), s.OrganizationID))
		c.Request = c.Request.WithContext(insight.WithOrganization(c.Request.Context(), s.OrganizationID))
		c.Request = c.Request.WithContext(agentrun.WithOrganization(c.Request.Context(), s.OrganizationID))
		c.Next()
	}
}
func OrganizationID(c *gin.Context) (string, bool) {
	v, ok := c.Get("organization_id")
	s, valid := v.(string)
	return s, ok && valid && s != ""
}
