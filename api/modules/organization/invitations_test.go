package organization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func invitationRequest(t *testing.T, router http.Handler, actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, router, actor, method, path, body)
}

func acceptRequest(t *testing.T, router http.Handler, actor, token string) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, router, actor, "POST", "/../organization-invitations/accept", `{"token":"`+token+`"}`)
}

func TestInvitationHTTPAuthorizationExpiryReplayAndHashStorage(t *testing.T) {
	db := integrationDB(t)
	router := testRouter(db)
	repo := NewRepository(db)
	owner, admin, member, viewer, outsider, foreign := testAccount(t, db), testAccount(t, db), testAccount(t, db), testAccount(t, db), testAccount(t, db), testAccount(t, db)
	org, err := repo.Create(context.Background(), owner, "Invitations")
	require.NoError(t, err)
	foreignOrg, err := repo.Create(context.Background(), foreign, "Foreign")
	require.NoError(t, err)
	require.NoError(t, db.Create(&Membership{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: admin, Role: RoleAdmin}).Error)
	require.NoError(t, db.Create(&Membership{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: member, Role: RoleMember}).Error)
	require.NoError(t, db.Create(&Membership{ID: uuid.NewString(), OrganizationID: org.ID, AccountID: viewer, Role: RoleViewer}).Error)

	for _, tc := range []struct {
		name, actor, role string
		want              int
	}{
		{"member denied", member, RoleViewer, http.StatusForbidden},
		{"viewer denied", viewer, RoleMember, http.StatusForbidden},
		{"outsider denied", outsider, RoleMember, http.StatusNotFound},
		{"foreign org denied", owner, RoleMember, http.StatusNotFound},
		{"owner role rejected", owner, RoleOwner, http.StatusBadRequest},
		{"invalid role rejected", owner, "bogus", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/" + org.ID + "/invitations"
			if tc.name == "foreign org denied" {
				path = "/" + foreignOrg.ID + "/invitations"
			}
			w := invitationRequest(t, router, tc.actor, "POST", path, `{"role":"`+tc.role+`","expires_in_seconds":60}`)
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}

	w := invitationRequest(t, router, owner, "POST", "/"+org.ID+"/invitations", `{"role":"member","expires_in_seconds":60}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		Data CreateInvitationResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.Token)
	digest := sha256.Sum256([]byte(created.Data.Token))
	require.NotContains(t, w.Body.String(), hex.EncodeToString(digest[:]))
	var stored Invitation
	require.NoError(t, db.Where("id = ?", created.Data.Invitation.ID).First(&stored).Error)
	require.Equal(t, sha256.Sum256([]byte(created.Data.Token)), mustHash(stored.TokenHash))

	accepted := acceptRequest(t, router, outsider, created.Data.Token)
	require.Equal(t, http.StatusCreated, accepted.Code, accepted.Body.String())
	require.Equal(t, http.StatusConflict, acceptRequest(t, router, outsider, created.Data.Token).Code)
	require.Equal(t, http.StatusConflict, acceptRequest(t, router, foreign, created.Data.Token).Code)

	expired, token, err := repo.CreateInvitation(context.Background(), org.ID, owner, RoleViewer, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.NoError(t, db.Model(&Invitation{}).Where("id = ?", expired.ID).Update("expires_at", time.Now().Add(-time.Second)).Error)
	require.Equal(t, http.StatusConflict, acceptRequest(t, router, foreign, token).Code)

	future, token, err := repo.CreateInvitation(context.Background(), org.ID, owner, RoleViewer, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.NotNil(t, future)
	require.Equal(t, http.StatusCreated, acceptRequest(t, router, foreign, token).Code)
}

func mustHash(value string) [32]byte {
	var out [32]byte
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(out) {
		return out
	}
	copy(out[:], decoded)
	return out
}

func TestInvitationConcurrentAcceptanceConsumesOnce(t *testing.T) {
	db := integrationDB(t)
	repo := NewRepository(db)
	owner, first, second := testAccount(t, db), testAccount(t, db), testAccount(t, db)
	org, err := repo.Create(context.Background(), owner, "Concurrent invitations")
	require.NoError(t, err)
	_, token, err := repo.CreateInvitation(context.Background(), org.ID, owner, RoleMember, time.Now().Add(time.Minute))
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, account := range []string{first, second} {
		wg.Add(1)
		go func(account string) {
			defer wg.Done()
			_, err := repo.AcceptInvitation(context.Background(), account, token)
			results <- err
		}(account)
	}
	wg.Wait()
	firstErr, secondErr := <-results, <-results
	require.True(t, (firstErr == nil) != (secondErr == nil), "exactly one concurrent acceptance must succeed: %v / %v", firstErr, secondErr)
	if firstErr != nil {
		require.ErrorIs(t, firstErr, ErrInvitationInvalid)
	} else {
		require.ErrorIs(t, secondErr, ErrInvitationInvalid)
	}
}
