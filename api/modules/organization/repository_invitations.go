package organization

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvitationInvalid = errors.New("invalid invitation")

func invitationToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate invitation token: %w", err)
	}
	token := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}

func (r *Repository) CreateInvitation(ctx context.Context, orgID, actor, role string, expiresAt time.Time) (*Invitation, string, error) {
	if role == RoleOwner || !validRole(role) {
		return nil, "", ErrForbidden
	}
	if !expiresAt.After(time.Now()) {
		return nil, "", ErrInvitationInvalid
	}
	token, hash, err := invitationToken()
	if err != nil {
		return nil, "", err
	}
	var out Invitation
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&org, "id = ?", orgID).Error; err != nil {
			return err
		}
		var member Membership
		if err := tx.Where("organization_id=? AND account_id=?", orgID, actor).First(&member).Error; err != nil {
			return ErrNotMember
		}
		if !canManage(member.Role, role) {
			return ErrForbidden
		}
		out = Invitation{ID: uuid.NewString(), OrganizationID: orgID, InviterAccountID: actor, TokenHash: hash, Role: role, ExpiresAt: expiresAt}
		return tx.Create(&out).Error
	})
	return &out, token, err
}

func (r *Repository) AcceptInvitation(ctx context.Context, accountID, token string) (*Membership, error) {
	hash := sha256.Sum256([]byte(token))
	var membership Membership
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invite Invitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hex.EncodeToString(hash[:])).First(&invite).Error; err != nil {
			return ErrInvitationInvalid
		}
		if invite.AcceptedAt != nil || !invite.ExpiresAt.After(time.Now()) {
			return ErrInvitationInvalid
		}
		var existing Membership
		if err := tx.Where("organization_id=? AND account_id=?", invite.OrganizationID, accountID).First(&existing).Error; err == nil {
			return ErrInvitationInvalid
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		membership = Membership{ID: uuid.NewString(), OrganizationID: invite.OrganizationID, AccountID: accountID, Role: invite.Role}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("create membership: %w", err)
		}
		now := time.Now()
		return tx.Model(&invite).Updates(map[string]any{"accepted_at": now, "accepted_by_account_id": accountID}).Error
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, ErrInvitationInvalid
	}
	if err != nil {
		return nil, err
	}
	return &membership, nil
}
