package alert

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const thresholdOrg = "00000000-0000-4000-8000-000000000001"

func thresholdRouter(db *gorm.DB, org, account, role string, authenticated bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set("organization_id", org)
			c.Set("account_id", account)
			c.Next()
		})
	}
	NewRoutes(NewController(NewRepository(db), organization.NewRepository(db))).Register(r.Group(""))
	return r
}

func expectMembership(mock sqlmock.Sqlmock, org, account, role string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2 ORDER BY "organization_memberships"."id" LIMIT $3`)).
		WithArgs(org, account, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "role"}).AddRow("membership", org, account, role))
}

func sendThresholdRequest(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	return resp.Code, payload
}

func TestThresholdRoutesGetDefaultsAndPersistedValues(t *testing.T) {
	db, mock := thresholdDB(t)
	expectMembership(mock, thresholdOrg, "member", organization.RoleMember)
	mock.ExpectQuery(`SELECT \* FROM "alert_thresholds"`).WithArgs(thresholdOrg, 1).WillReturnError(gorm.ErrRecordNotFound)
	code, payload := sendThresholdRequest(t, thresholdRouter(db, thresholdOrg, "member", organization.RoleMember, true), http.MethodGet, "/alerts/thresholds", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, map[string]any{"min_cache_hit": .8, "silent_seconds": float64(300), "pr_stale_seconds": float64(86400)}, payload["data"])

	orgB := "00000000-0000-4000-8000-000000000002"
	expectMembership(mock, orgB, "viewer", organization.RoleViewer)
	mock.ExpectQuery(`SELECT \* FROM "alert_thresholds"`).WithArgs(orgB, 1).WillReturnRows(sqlmock.NewRows([]string{"organization_id", "min_cache_hit", "silent_seconds", "stale_seconds"}).AddRow(orgB, .61, 17, 901))
	code, payload = sendThresholdRequest(t, thresholdRouter(db, orgB, "viewer", organization.RoleViewer, true), http.MethodGet, "/alerts/thresholds", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, map[string]any{"min_cache_hit": .61, "silent_seconds": float64(17), "pr_stale_seconds": float64(901)}, payload["data"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestThresholdRoutesUpdateReturnsPersistedValuesAndScopesWrite(t *testing.T) {
	db, mock := thresholdDB(t)
	expectMembership(mock, thresholdOrg, "owner", organization.RoleOwner)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO alert_thresholds (organization_id, min_cache_hit, silent_seconds, stale_seconds)
		VALUES ($1, $2, $3, $4) ON CONFLICT (organization_id) DO UPDATE SET
		min_cache_hit = EXCLUDED.min_cache_hit, silent_seconds = EXCLUDED.silent_seconds, stale_seconds = EXCLUDED.stale_seconds, updated_at = NOW()`)).
		WithArgs(thresholdOrg, .73, int64(41), int64(902)).WillReturnResult(sqlmock.NewResult(0, 1))
	code, payload := sendThresholdRequest(t, thresholdRouter(db, thresholdOrg, "owner", organization.RoleOwner, true), http.MethodPut, "/alerts/thresholds", `{"min_cache_hit":0.73,"silent_seconds":41,"pr_stale_seconds":902}`)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, map[string]any{"min_cache_hit": .73, "silent_seconds": float64(41), "pr_stale_seconds": float64(902)}, payload["data"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestThresholdRoutesAuthRolesValidationAndDatabaseErrors(t *testing.T) {
	tests := []struct {
		name, role string
		status     int
	}{
		{"viewer cannot write", organization.RoleViewer, http.StatusForbidden},
		{"member cannot write", organization.RoleMember, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock := thresholdDB(t)
			expectMembership(mock, thresholdOrg, "account", tt.role)
			code, _ := sendThresholdRequest(t, thresholdRouter(db, thresholdOrg, "account", tt.role, true), http.MethodPut, "/alerts/thresholds", `{"min_cache_hit":0.5,"silent_seconds":1,"pr_stale_seconds":1}`)
			require.Equal(t, tt.status, code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}

	db, mock := thresholdDB(t)
	code, _ := sendThresholdRequest(t, thresholdRouter(db, thresholdOrg, "", "", false), http.MethodGet, "/alerts/thresholds", "")
	require.Equal(t, http.StatusUnauthorized, code)
	require.NoError(t, mock.ExpectationsWereMet())

	db, mock = thresholdDB(t)
	for _, body := range []string{`{"min_cache_hit":-0.01,"silent_seconds":1,"pr_stale_seconds":1}`, `{"min_cache_hit":1.01,"silent_seconds":1,"pr_stale_seconds":1}`, `{"min_cache_hit":0.5,"silent_seconds":0,"pr_stale_seconds":1}`, `{"min_cache_hit":0.5,"silent_seconds":1,"pr_stale_seconds":9223372036854775807}`} {
		expectMembership(mock, thresholdOrg, "owner", organization.RoleOwner)
		code, _ = sendThresholdRequest(t, thresholdRouter(db, thresholdOrg, "owner", organization.RoleOwner, true), http.MethodPut, "/alerts/thresholds", body)
		require.Equal(t, http.StatusBadRequest, code)
	}
	require.NoError(t, mock.ExpectationsWereMet())

	db, mock = thresholdDB(t)
	expectMembership(mock, thresholdOrg, "owner", organization.RoleOwner)
	mock.ExpectQuery(`SELECT \* FROM "alert_thresholds"`).WithArgs(thresholdOrg, 1).WillReturnError(sqlmock.ErrCancelled)
	code, _ = sendThresholdRequest(t, thresholdRouter(db, thresholdOrg, "owner", organization.RoleOwner, true), http.MethodGet, "/alerts/thresholds", "")
	require.Equal(t, http.StatusInternalServerError, code)
	require.NoError(t, mock.ExpectationsWereMet())
}
