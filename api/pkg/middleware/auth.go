package middleware

import (
	"net/http"
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

const SessionKey = "auth.session"

func RequireSession(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok && isWebSocketUpgrade(c.Request) {
			token = c.Query("access_token")
			ok = token != ""
		}
		if !ok {
			response.Unauthorized(c, "invalid token")
			c.Abort()
			return
		}
		s, err := service.Session(token)
		if err != nil || s.OrganizationID == "" {
			response.Unauthorized(c, "organization selection required")
			c.Abort()
			return
		}
		c.Set(SessionKey, s)
		c.Next()
	}
}

func bearerToken(value string) (string, bool) {
	p := strings.Fields(value)
	return func() (string, bool) {
		if len(p) != 2 || !strings.EqualFold(p[0], "Bearer") || p[1] == "" {
			return "", false
		}
		return p[1], true
	}()
}

func isWebSocketUpgrade(request *http.Request) bool {
	return strings.EqualFold(request.Header.Get("Upgrade"), "websocket") &&
		hasToken(request.Header.Get("Connection"), "upgrade")
}

func hasToken(value, wanted string) bool {
	for _, token := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(token), wanted) {
			return true
		}
	}
	return false
}
func Session(c *gin.Context) (*auth.SessionResponse, bool) {
	v, ok := c.Get(SessionKey)
	s, valid := v.(*auth.SessionResponse)
	return s, ok && valid && s != nil
}
