package repositorysettings

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type manualSyncStub struct {
	err   error
	calls int
}

func (s *manualSyncStub) Sync(context.Context, store.UUID) error {
	s.calls++
	return s.err
}

func TestManualSyncSuccessReturnsReloadedSafeStatus(t *testing.T) {
	controller, mock, syncer := manualController(t, nil)
	expectMembership(mock, "org-a", "acct-a", organization.RoleMember)
	expectScopedRepository(mock, "org-a", "repo-a")
	expectReloadedRepository(mock, "repo-a", "success", "")

	w := performManualSync(controller, "acct-a", "org-a", "repo-a")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"last_sync_status":"success"`)
	require.NotContains(t, w.Body.String(), "bearer-test-secret")
	require.Equal(t, 1, syncer.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestManualSyncFailureDoesNotReturnSecrets(t *testing.T) {
	controller, mock, syncer := manualController(t, errors.New("Bearer bearer-test-secret https://url-user:url-password@example.test webhook-test-secret"))
	expectMembership(mock, "org-a", "acct-a", organization.RoleOwner)
	expectScopedRepository(mock, "org-a", "repo-a")

	w := performManualSync(controller, "acct-a", "org-a", "repo-a")
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "repository sync failed")
	for _, secret := range []string{"bearer-test-secret", "url-password", "webhook-test-secret"} {
		require.NotContains(t, w.Body.String(), secret)
	}
	require.Equal(t, 1, syncer.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestManualSyncHandlesStatusReloadFailure(t *testing.T) {
	controller, mock, _ := manualController(t, nil)
	expectMembership(mock, "org-a", "acct-a", organization.RoleAdmin)
	expectScopedRepository(mock, "org-a", "repo-a")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE id = $1 ORDER BY "repositories"."id" LIMIT $2`)).
		WithArgs(store.UUID("repo-a"), 1).WillReturnError(errors.New("reload failed"))

	w := performManualSync(controller, "acct-a", "org-a", "repo-a")
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "failed to load repository sync status")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestManualSyncEnforcesMembershipAndTenantIsolation(t *testing.T) {
	t.Run("non-member", func(t *testing.T) {
		controller, mock, syncer := manualController(t, nil)
		mock.ExpectQuery(`SELECT \* FROM "organization_memberships"`).WithArgs("org-a", "outsider", 1).WillReturnError(gorm.ErrRecordNotFound)
		w := performManualSync(controller, "outsider", "org-a", "repo-a")
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		require.Zero(t, syncer.calls)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("other tenant", func(t *testing.T) {
		controller, mock, syncer := manualController(t, nil)
		expectMembership(mock, "org-b", "acct-b", organization.RoleViewer)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE id = $1 AND organization_id = $2 ORDER BY "repositories"."id" LIMIT $3`)).
			WithArgs(store.UUID("repo-a"), store.UUID("org-b"), 1).WillReturnError(gorm.ErrRecordNotFound)
		w := performManualSync(controller, "acct-b", "org-b", "repo-a")
		require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		require.Zero(t, syncer.calls)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func manualController(t *testing.T, syncError error) (*Controller, sqlmock.Sqlmock, *manualSyncStub) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	syncer := &manualSyncStub{err: syncError}
	return NewController(store.New(db).Repositories, organization.NewRepository(db), nil, "", syncer), mock, syncer
}

func expectMembership(mock sqlmock.Sqlmock, org, account, role string) {
	mock.ExpectQuery(`SELECT \* FROM "organization_memberships"`).WithArgs(org, account, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "role"}).AddRow("membership", org, account, role))
}

func expectScopedRepository(mock sqlmock.Sqlmock, org, id string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE id = $1 AND organization_id = $2 ORDER BY "repositories"."id" LIMIT $3`)).
		WithArgs(store.UUID(id), store.UUID(org), 1).WillReturnRows(repositoryStatusRows(id, org, "", ""))
}

func expectReloadedRepository(mock sqlmock.Sqlmock, id, status, syncError string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE id = $1 ORDER BY "repositories"."id" LIMIT $2`)).
		WithArgs(store.UUID(id), 1).WillReturnRows(repositoryStatusRows(id, "org-a", status, syncError))
}

func repositoryStatusRows(id, org, status, syncError string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "organization_id", "provider", "full_name", "status_mappings", "last_sync_status", "last_sync_error"}).
		AddRow(id, org, "github", "acme/app", []byte(`{}`), status, syncError)
}

func performManualSync(controller *Controller, account, org, id string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		ctx.Set("account_id", account)
		ctx.Set("organization_id", org)
	})
	NewRoutes(controller).Register(router.Group("/api/v1"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/repositories/"+id+"/sync", nil))
	return w
}
