package insight

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(controller *Controller) *Routes { return &Routes{controller: controller} }
func (r *Routes) Register(group *gin.RouterGroup) {
	group.GET("/insights", r.controller.Get)
	group.GET("/insights/analytics", r.controller.Analytics)
}
