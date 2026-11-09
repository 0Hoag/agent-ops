package work

import "github.com/gin-gonic/gin"

type Routes struct{ c *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{c: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	g.GET("/work", r.c.List)
	g.GET("/work/:kind/:number", r.c.Detail)
}
