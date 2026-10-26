package organization

import "github.com/gin-gonic/gin"

type Routes struct{ controller *Controller }

func NewRoutes(c *Controller) *Routes { return &Routes{controller: c} }
func (r *Routes) Register(g *gin.RouterGroup) {
	o := g.Group("/organizations")
	o.POST("", r.controller.Create)
	o.GET("", r.controller.List)
	o.GET("/:organization_id", r.controller.Detail)
	o.GET("/:organization_id/members", r.controller.Members)
	o.POST("/:organization_id/members", r.controller.Add)
	o.PATCH("/:organization_id/members/:account_id", r.controller.Role)
	o.DELETE("/:organization_id/members/:account_id", r.controller.Remove)
	o.POST("/:organization_id/invitations", r.controller.CreateInvitation)
	g.POST("/organization-invitations/accept", r.controller.AcceptInvitation)
}
