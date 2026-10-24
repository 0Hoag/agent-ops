package webhook

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(controller *Controller) *Routes { return &Routes{controller: controller} }
func (routes *Routes) Register(group *gin.RouterGroup) {
	group.POST("/webhooks/:provider", routes.controller.Handle)
}
