package organization

import "time"

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

type Organization struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Organization) TableName() string { return "organizations" }

type Membership struct {
	ID             string    `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID string    `json:"organization_id" gorm:"type:uuid;not null"`
	AccountID      string    `json:"account_id" gorm:"type:uuid;not null"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Membership) TableName() string { return "organization_memberships" }
