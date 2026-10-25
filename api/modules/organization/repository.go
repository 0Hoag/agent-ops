package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotMember = errors.New("not a member")
var ErrForbidden = errors.New("forbidden")
var ErrLastOwner = errors.New("cannot remove or demote the last owner")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func validRole(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember || role == RoleViewer
}
func canManage(role, target string) bool {
	return role == RoleOwner || (role == RoleAdmin && (target == RoleMember || target == RoleViewer))
}

func (r *Repository) Create(ctx context.Context, accountID, name string) (*Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	item := &Organization{ID: uuid.NewString(), Name: name}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(item).Error; err != nil {
			return fmt.Errorf("create organization: %w", err)
		}
		if err := tx.Create(&Membership{ID: uuid.NewString(), OrganizationID: item.ID, AccountID: accountID, Role: RoleOwner}).Error; err != nil {
			return fmt.Errorf("grant owner membership: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}
func (r *Repository) Membership(ctx context.Context, orgID, accountID string) (*Membership, error) {
	var m Membership
	err := r.db.WithContext(ctx).Where("organization_id = ? AND account_id = ?", orgID, accountID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("find membership: %w", err)
	}
	return &m, nil
}
func (r *Repository) List(ctx context.Context, accountID string) ([]OrganizationDetail, error) {
	var rows []OrganizationDetail
	err := r.db.WithContext(ctx).Table("organizations o").Select("o.*, m.id as membership_id, m.organization_id as membership_organization_id, m.account_id as membership_account_id, m.role as membership_role, m.created_at as membership_created_at, m.updated_at as membership_updated_at").Joins("JOIN organization_memberships m ON m.organization_id=o.id AND m.account_id=?", accountID).Scan(&rows).Error
	return rows, err
}
func (r *Repository) Detail(ctx context.Context, orgID, accountID string) (*OrganizationDetail, error) {
	m, err := r.Membership(ctx, orgID, accountID)
	if err != nil {
		return nil, err
	}
	var o Organization
	if err := r.db.WithContext(ctx).First(&o, "id = ?", orgID).Error; err != nil {
		return nil, err
	}
	return &OrganizationDetail{Organization: o, Membership: *m}, nil
}
func (r *Repository) Members(ctx context.Context, orgID, accountID string) ([]Membership, error) {
	if _, err := r.Membership(ctx, orgID, accountID); err != nil {
		return nil, err
	}
	var out []Membership
	err := r.db.WithContext(ctx).Where("organization_id = ?", orgID).Order("created_at").Find(&out).Error
	return out, err
}
func (r *Repository) Add(ctx context.Context, orgID, actor, target, role string) error {
	return r.locked(ctx, orgID, func(locked *Repository) error {
		return locked.add(ctx, orgID, actor, target, role)
	})
}
func (r *Repository) add(ctx context.Context, orgID, actor, target, role string) error {
	if !validRole(role) || role == RoleOwner {
		return ErrForbidden
	}
	a, err := r.Membership(ctx, orgID, actor)
	if err != nil {
		return err
	}
	if !canManage(a.Role, role) {
		return ErrForbidden
	}
	return r.db.WithContext(ctx).Create(&Membership{ID: uuid.NewString(), OrganizationID: orgID, AccountID: target, Role: role}).Error
}
func (r *Repository) UpdateRole(ctx context.Context, orgID, actor, target, role string) error {
	return r.locked(ctx, orgID, func(locked *Repository) error {
		return locked.updateRole(ctx, orgID, actor, target, role)
	})
}
func (r *Repository) updateRole(ctx context.Context, orgID, actor, target, role string) error {
	if !validRole(role) || role == RoleOwner {
		return ErrForbidden
	}
	a, err := r.Membership(ctx, orgID, actor)
	if err != nil {
		return err
	}
	if !canManage(a.Role, role) {
		return ErrForbidden
	}
	var current Membership
	if err := r.db.WithContext(ctx).Where("organization_id=? AND account_id=?", orgID, target).First(&current).Error; err != nil {
		return err
	}
	if !canManage(a.Role, current.Role) {
		return ErrForbidden
	}
	if current.Role == RoleOwner && role != RoleOwner {
		if err := r.ensureNotLastOwner(ctx, orgID); err != nil {
			return err
		}
	}
	return r.db.WithContext(ctx).Model(&current).Update("role", role).Error
}
func (r *Repository) Remove(ctx context.Context, orgID, actor, target string) error {
	return r.locked(ctx, orgID, func(locked *Repository) error {
		return locked.remove(ctx, orgID, actor, target)
	})
}
func (r *Repository) remove(ctx context.Context, orgID, actor, target string) error {
	a, err := r.Membership(ctx, orgID, actor)
	if err != nil {
		return err
	}
	var m Membership
	if err := r.db.WithContext(ctx).Where("organization_id=? AND account_id=?", orgID, target).First(&m).Error; err != nil {
		return err
	}
	if !canManage(a.Role, m.Role) || m.Role == RoleOwner && a.Role != RoleOwner {
		return ErrForbidden
	}
	if m.Role == RoleOwner {
		if err := r.ensureNotLastOwner(ctx, orgID); err != nil {
			return err
		}
	}
	return r.db.WithContext(ctx).Delete(&m).Error
}
func (r *Repository) ensureNotLastOwner(ctx context.Context, orgID string) error {
	var n int64
	if err := r.db.WithContext(ctx).Model(&Membership{}).Where("organization_id=? AND role=?", orgID, RoleOwner).Count(&n).Error; err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastOwner
	}
	return nil
}

// Serialize membership mutations so simultaneous owner changes cannot remove all owners.
func (r *Repository) locked(ctx context.Context, orgID string, change func(*Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&org, "id = ?", orgID).Error; err != nil {
			return err
		}
		return change(NewRepository(tx))
	})
}
