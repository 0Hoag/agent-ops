package worker

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{controller: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	w := g.Group("/workers")
	w.POST("/start", r.controller.Start)
	w.POST("/restart", r.controller.Restart)
	w.POST("/:worker_id/stop", r.controller.Stop)
	w.GET("/:worker_id", r.controller.Get)
	w.GET("/:worker_id/logs", r.controller.Logs)
}
