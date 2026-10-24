package agentrun

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{controller: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	g.POST("/agent-runs", r.controller.Create)
	g.GET("/agent-runs", r.controller.List)
	g.GET("/agent-runs/aggregate", r.controller.Aggregate)
}
