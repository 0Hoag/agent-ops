package wave

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{controller: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	w := g.Group("/waves")
	w.GET("", r.controller.List)
	w.POST("", r.controller.Create)
	w.GET("/:wave_id", r.controller.Get)
	w.PATCH("/:wave_id", r.controller.Update)
	w.DELETE("/:wave_id", r.controller.Delete)
}
