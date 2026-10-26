package organization

import "time"

type Invitation struct {
	ID                  string     `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID      string     `json:"organization_id" gorm:"type:uuid;not null"`
	InviterAccountID    string     `json:"-" gorm:"type:uuid;not null"`
	TokenHash           string     `json:"-" gorm:"not null;uniqueIndex"`
	Role                string     `json:"role"`
	ExpiresAt           time.Time  `json:"expires_at"`
	AcceptedAt          *time.Time `json:"accepted_at,omitempty"`
	AcceptedByAccountID *string    `json:"-" gorm:"type:uuid"`
	CreatedAt           time.Time  `json:"created_at"`
}

func (Invitation) TableName() string { return "organization_invitations" }

type CreateInvitationRequest struct {
	Role             string `json:"role" binding:"required"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

type CreateInvitationResponse struct {
	Invitation Invitation `json:"invitation"`
	Token      string     `json:"token"`
	Link       string     `json:"link"`
}

type AcceptInvitationRequest struct {
	Token string `json:"token" binding:"required"`
}
