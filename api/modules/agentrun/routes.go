package agentrun

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{controller: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	g.POST("/agent-runs", r.controller.Create)
	g.POST("/agent-runs/:run_id/timeline", r.controller.IngestTimeline)
}

// RegisterPrivate exposes read operations only after organization middleware has set the tenant.
func (r *Routes) RegisterPrivate(g *gin.RouterGroup) {
	g.GET("/agent-runs", r.controller.List)
	g.GET("/agent-runs/:run_id/timeline", r.controller.Timeline)
}

func (r *Routes) RegisterAggregate(g *gin.RouterGroup) {
	g.GET("/agent-runs/aggregate", r.controller.Aggregate)
}

func RegisterPriceRoutes(g *gin.RouterGroup, c *PriceController) {
	p := g.Group("/agent-run-prices")
	p.GET("", c.List)
	p.POST("", c.Create)
	p.PATCH("/:price_id", c.Update)
}
