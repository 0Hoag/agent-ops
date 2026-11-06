package budget

import "github.com/gin-gonic/gin"

type Routes struct{ c *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{c: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	x := g.Group("/budgets")
	x.GET("", r.c.List)
	x.POST("", r.c.Create)
	x.GET("/:budget_id", r.c.Get)
	x.PATCH("/:budget_id", r.c.Update)
	x.DELETE("/:budget_id", r.c.Delete)
}
