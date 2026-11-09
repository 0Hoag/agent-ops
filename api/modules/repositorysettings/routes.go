package repositorysettings

import "github.com/gin-gonic/gin"

type Routes struct{ c *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{c: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	x := g.Group("/repositories")
	x.POST("/test", r.c.Test)
	x.GET("", r.c.List)
	x.POST("", r.c.Create)
	x.GET("/:repository_id", r.c.Get)
	x.PATCH("/:repository_id", r.c.Update)
	x.DELETE("/:repository_id", r.c.Delete)
	x.POST("/:repository_id/sync", r.c.Sync)
}
