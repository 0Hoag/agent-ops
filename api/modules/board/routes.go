package board

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(controller *Controller) *Routes { return &Routes{controller: controller} }
func (r *Routes) Register(group *gin.RouterGroup) {
	group.GET("/repositories", r.controller.ListRepositories)
	group.GET("/board", r.controller.ListBoard)
}
