package organization

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOrganizationHTTPPermissionMatrix(t *testing.T) {
	db := integrationDB(t)
	router, repo := testRouter(db), NewRepository(db)
	ctx := context.Background()
	for _, actorRole := range []string{RoleOwner, RoleAdmin, RoleMember, RoleViewer, "outsider"} {
		for _, targetRole := range []string{RoleOwner, RoleAdmin, RoleMember, RoleViewer} {
			for _, action := range []string{"add", "update", "remove"} {
				t.Run(actorRole+"/"+targetRole+"/"+action, func(t *testing.T) {
					owner, actor, target := testAccount(t, db), testAccount(t, db), testAccount(t, db)
					org, err := repo.Create(ctx, owner, "Matrix")
					require.NoError(t, err)
					if actorRole != "outsider" {
						require.NoError(t, db.Create(&Membership{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: actor, Role: actorRole}).Error)
					} else {
						_, err := repo.Create(ctx, actor, "Other organization")
						require.NoError(t, err)
					}
					readStatus := 200
					if actorRole == "outsider" {
						readStatus = 404
					}
					for _, suffix := range []string{"", "/members"} {
						require.Equal(t, readStatus, request(t, router, actor, "GET", "/"+org.ID+suffix, "").Code)
					}
					method, path := "POST", "/"+org.ID+"/members"
					role := targetRole
					if action != "add" {
						require.NoError(t, db.Create(&Membership{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: target, Role: targetRole}).Error)
						path += "/" + target
						method, role = "PATCH", RoleViewer
						if action == "remove" {
							method = "DELETE"
						}
					}
					// Independent expected policy, not the production canManage helper.
					allowed := actorRole == RoleOwner || actorRole == RoleAdmin && (targetRole == RoleMember || targetRole == RoleViewer)
					if action == "add" && targetRole == RoleOwner {
						allowed = false
					}
					want := 403
					if allowed {
						want = 204
					}
					if actorRole == "outsider" {
						want = 404
					}
					w := request(t, router, actor, method, path, fmt.Sprintf(`{"account_id":%q,"role":%q}`, target, role))
					// Owner assignment is rejected before identity lookup.
					if actorRole == "outsider" && action == "add" && targetRole == RoleOwner {
						want = 403
					}
					require.Equal(t, want, w.Code, w.Body.String())
					var count int64
					require.NoError(t, db.Model(&Membership{}).Where("organization_id=? AND account_id=?", org.ID, target).Count(&count).Error)
					if action == "add" {
						if allowed {
							require.EqualValues(t, 1, count)
						} else {
							require.Zero(t, count)
						}
					} else if action == "remove" && allowed {
						require.Zero(t, count)
					} else {
						m, err := repo.Membership(ctx, org.ID, target)
						require.NoError(t, err)
						wantRole := targetRole
						if action == "update" && allowed {
							wantRole = RoleViewer
						}
						require.Equal(t, wantRole, m.Role)
					}
				})
			}
		}
	}
}

func TestOrganizationRepositoryConstraints(t *testing.T) {
	db := integrationDB(t)
	repo, ctx := NewRepository(db), context.Background()
	owner, target := testAccount(t, db), testAccount(t, db)
	org, err := repo.Create(ctx, owner, "Constraints")
	require.NoError(t, err)
	require.ErrorIs(t, repo.Remove(ctx, org.ID, owner, owner), ErrLastOwner)
	require.ErrorIs(t, repo.UpdateRole(ctx, org.ID, owner, owner, RoleMember), ErrLastOwner)
	require.NoError(t, repo.Add(ctx, org.ID, owner, target, RoleMember))
	require.Error(t, repo.Add(ctx, org.ID, owner, target, RoleViewer))
	for _, role := range []string{RoleOwner, "root", ""} {
		require.ErrorIs(t, repo.Add(ctx, org.ID, owner, uuid.NewString(), role), ErrForbidden)
		require.ErrorIs(t, repo.UpdateRole(ctx, org.ID, owner, target, role), ErrForbidden)
	}
	var before, after int64
	require.NoError(t, db.Model(&Organization{}).Count(&before).Error)
	_, err = repo.Create(ctx, uuid.NewString(), "Must rollback")
	require.Error(t, err)
	require.NoError(t, db.Model(&Organization{}).Count(&after).Error)
	require.Equal(t, before, after)
	for _, membership := range []Membership{
		{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: testAccount(t, db), Role: "root"},
		{ID: uuid.NewString(), OrganizationID: uuid.NewString(), AccountID: target, Role: RoleMember},
		{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: uuid.NewString(), Role: RoleMember},
	} {
		require.Error(t, db.Create(&membership).Error)
	}
	// A second owner is seeded directly: callers can never assign owner.
	second := testAccount(t, db)
	require.NoError(t, db.Create(&Membership{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: second, Role: RoleOwner}).Error)
	require.NoError(t, repo.UpdateRole(ctx, org.ID, owner, second, RoleMember))
	m, err := repo.Membership(ctx, org.ID, second)
	require.NoError(t, err)
	require.Equal(t, RoleMember, m.Role)
}
