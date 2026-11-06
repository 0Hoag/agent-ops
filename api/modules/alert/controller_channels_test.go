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
	"github.com/stretchr/testify/require"
)

const channelOrg = "00000000-0000-4000-8000-000000000001"

func sendChannelRequest(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	return resp.Code, payload
}

func channelMembership(mock sqlmock.Sqlmock, org, account, role string) {
	expectMembership(mock, org, account, role)
}

func TestChannelRoutesCRUDAndTenantIsolation(t *testing.T) {
	db, mock := thresholdDB(t)
	r := thresholdRouter(db, channelOrg, "owner", organization.RoleOwner, true)
	channelMembership(mock, channelOrg, "owner", organization.RoleOwner)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO alert_channels (id, organization_id, kind, enabled, telegram_token_env_ref, telegram_chat_id_env_ref, slack_webhook_url_env_ref, smtp_host_env_ref, smtp_port_env_ref, smtp_username_env_ref, smtp_password_env_ref, smtp_from_env_ref, smtp_to_env_ref) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`)).
		WithArgs(sqlmock.AnyArg(), channelOrg, "telegram", true, "NOTIFY_TOKEN", "NOTIFY_CHAT", "", "", "", "", "", "", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	code, payload := sendChannelRequest(t, r, http.MethodPost, "/alerts/channels", `{"kind":"telegram","enabled":true,"telegram_token_env_ref":"NOTIFY_TOKEN","telegram_chat_id_env_ref":"NOTIFY_CHAT"}`)
	require.Equal(t, http.StatusCreated, code)
	require.Equal(t, "telegram", payload["data"].(map[string]any)["kind"])
	require.Equal(t, "NOTIFY_TOKEN", payload["data"].(map[string]any)["telegram_token_env_ref"])
	require.NoError(t, mock.ExpectationsWereMet())

	db, mock = thresholdDB(t)
	r = thresholdRouter(db, channelOrg, "member", organization.RoleMember, true)
	channelMembership(mock, channelOrg, "member", organization.RoleMember)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "alert_channels" WHERE organization_id = $1 ORDER BY kind ASC`)).
		WithArgs(channelOrg).WillReturnRows(sqlmock.NewRows([]string{"organization_id", "kind", "enabled", "telegram_token_env_ref", "telegram_chat_id_env_ref"}).AddRow(channelOrg, "telegram", true, "NOTIFY_TOKEN", "NOTIFY_CHAT"))
	code, payload = sendChannelRequest(t, r, http.MethodGet, "/alerts/channels", "")
	require.Equal(t, http.StatusOK, code)
	rows := payload["data"].([]any)
	require.Len(t, rows, 1)
	require.Equal(t, "NOTIFY_TOKEN", rows[0].(map[string]any)["telegram_token_env_ref"])
	require.NoError(t, mock.ExpectationsWereMet())

	db, mock = thresholdDB(t)
	r = thresholdRouter(db, channelOrg, "member", organization.RoleMember, true)
	channelMembership(mock, channelOrg, "member", organization.RoleMember)
	code, _ = sendChannelRequest(t, r, http.MethodPut, "/alerts/channels/telegram", `{"enabled":false,"telegram_token_env_ref":"NOTIFY_TOKEN","telegram_chat_id_env_ref":"NOTIFY_CHAT"}`)
	require.Equal(t, http.StatusForbidden, code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelRoutesWriteScopesAndNeverResolveCredentials(t *testing.T) {
	db, mock := thresholdDB(t)
	r := thresholdRouter(db, channelOrg, "admin", organization.RoleAdmin, true)
	channelMembership(mock, channelOrg, "admin", organization.RoleAdmin)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE alert_channels SET enabled = $1, telegram_token_env_ref = $2, telegram_chat_id_env_ref = $3, slack_webhook_url_env_ref = $4, smtp_host_env_ref = $5, smtp_port_env_ref = $6, smtp_username_env_ref = $7, smtp_password_env_ref = $8, smtp_from_env_ref = $9, smtp_to_env_ref = $10, updated_at = NOW() WHERE organization_id = $11 AND kind = $12`)).
		WithArgs(false, "NOTIFY_TOKEN", "NOTIFY_CHAT", "", "", "", "", "", "", "", channelOrg, "telegram").
		WillReturnResult(sqlmock.NewResult(0, 1))
	code, payload := sendChannelRequest(t, r, http.MethodPut, "/alerts/channels/telegram", `{"enabled":false,"telegram_token_env_ref":"NOTIFY_TOKEN","telegram_chat_id_env_ref":"NOTIFY_CHAT"}`)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, false, payload["data"].(map[string]any)["enabled"])
	require.NotContains(t, string(mustJSON(t, payload)), "secret-value")
	require.NoError(t, mock.ExpectationsWereMet())

	db, mock = thresholdDB(t)
	r = thresholdRouter(db, channelOrg, "owner", organization.RoleOwner, true)
	channelMembership(mock, channelOrg, "owner", organization.RoleOwner)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM alert_channels WHERE organization_id = $1 AND kind = $2`)).WithArgs(channelOrg, "telegram").WillReturnResult(sqlmock.NewResult(0, 1))
	code, payload = sendChannelRequest(t, r, http.MethodDelete, "/alerts/channels/telegram", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "alert channel deleted", payload["message"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelRoutesRejectInvalidReferencesAndKindsBeforeDatabase(t *testing.T) {
	db, mock := thresholdDB(t)
	r := thresholdRouter(db, channelOrg, "owner", organization.RoleOwner, true)
	for _, body := range []string{
		`{"kind":"telegram","telegram_token_env_ref":"notify-token","telegram_chat_id_env_ref":"NOTIFY_CHAT"}`,
		`{"kind":"telegram","telegram_token_env_ref":"NOTIFY_TOKEN"}`,
		`{"kind":"webhook","enabled":true}`,
	} {
		expectMembership(mock, channelOrg, "owner", organization.RoleOwner)
		code, _ := sendChannelRequest(t, r, http.MethodPost, "/alerts/channels", body)
		require.Equal(t, http.StatusBadRequest, code)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	require.NoError(t, err)
	return b
}
