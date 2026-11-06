package alert

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(controller *Controller) *Routes { return &Routes{controller: controller} }
func (r *Routes) Register(group *gin.RouterGroup) {
	group.GET("/alerts/history", r.controller.History)
	group.GET("/alerts/thresholds", r.controller.GetThresholds)
	group.PUT("/alerts/thresholds", r.controller.UpdateThresholds)
}
