package handlers

import (
	"net/http"

	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
)

type Health struct{ db database.Pinger }

func NewHealth(db database.Pinger) *Health { return &Health{db: db} }

func (h *Health) Register(router *gin.Engine) {
	router.GET("/healthz", h.Healthz)
	router.GET("/readyz", h.Readyz)
}

// Healthz reports process health and does not depend on the database.
func (h *Health) Healthz(c *gin.Context) { response.Success(c, http.StatusOK, gin.H{"status": "ok"}) }

// Readyz reports whether the API can reach its database.
func (h *Health) Readyz(c *gin.Context) {
	if err := h.db.PingContext(c.Request.Context()); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	response.Success(c, http.StatusOK, gin.H{"status": "ok"})
}
