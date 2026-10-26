package auth

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(controller *Controller) *Routes { return &Routes{controller: controller} }
func (routes *Routes) Register(group *gin.RouterGroup) {
	group.POST("/auth/login", routes.controller.Login)
	group.GET("/auth/session", routes.controller.Session)
	group.POST("/auth/select-organization", routes.controller.SelectOrganization)
}
