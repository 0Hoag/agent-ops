package middleware

import (
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

const SessionKey = "auth.session"

func RequireSession(service *auth.Service) gin.HandlerFunc {
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
		c.Set(SessionKey, s)
		c.Next()
	}
}
func Session(c *gin.Context) (*auth.SessionResponse, bool) {
	v, ok := c.Get(SessionKey)
	s, valid := v.(*auth.SessionResponse)
	return s, ok && valid && s != nil
}
