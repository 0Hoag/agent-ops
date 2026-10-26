package organization

type CreateRequest struct {
	Name string `json:"name" binding:"required"`
}
type AddMemberRequest struct {
	AccountID string `json:"account_id" binding:"required"`
	Role      string `json:"role" binding:"required"`
}
type UpdateRoleRequest struct {
	Role string `json:"role" binding:"required"`
}
type OrganizationDetail struct {
	Organization
	Membership Membership `json:"membership" gorm:"embedded;embeddedPrefix:membership_"`
}
