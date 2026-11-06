package alert

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func testChannel(org string) Channel {
	return Channel{OrganizationID: org, Kind: "telegram", Enabled: true, TelegramTokenEnvRef: "NOTIFY_TOKEN", TelegramChatIDEnvRef: "NOTIFY_CHAT"}
}

func TestRepositoryCreateChannelPersistsReferencesAndTenant(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000001"
	insertSQL := `INSERT INTO alert_channels (id, organization_id, kind, enabled, telegram_token_env_ref, telegram_chat_id_env_ref, slack_webhook_url_env_ref, smtp_host_env_ref, smtp_port_env_ref, smtp_username_env_ref, smtp_password_env_ref, smtp_from_env_ref, smtp_to_env_ref) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(sqlmock.AnyArg(), org, "telegram", true, "NOTIFY_TOKEN", "NOTIFY_CHAT", "", "", "", "", "", "", "").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, NewRepository(db).CreateChannel(context.Background(), testChannel(org)))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryGetAndListChannelsAreOrganizationScoped(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000001"
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "alert_channels" WHERE organization_id = $1 AND kind = $2 ORDER BY "alert_channels"."id" LIMIT $3`)).WithArgs(org, "telegram", 1).WillReturnRows(sqlmock.NewRows([]string{"organization_id", "kind", "enabled", "telegram_token_env_ref"}).AddRow(org, "telegram", true, "NOTIFY_TOKEN"))
	got, err := NewRepository(db).GetChannel(context.Background(), org, "telegram")
	require.NoError(t, err)
	require.Equal(t, "NOTIFY_TOKEN", got.TelegramTokenEnvRef)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "alert_channels" WHERE organization_id = $1 ORDER BY kind ASC`)).WithArgs(org).WillReturnRows(sqlmock.NewRows([]string{"organization_id", "kind", "enabled"}).AddRow(org, "telegram", true))
	rows, err := NewRepository(db).ListChannels(context.Background(), org)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryUpdateAndDeleteChannelUseTenantPredicate(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000001"
	channel := testChannel(org)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE alert_channels SET enabled = $1, telegram_token_env_ref = $2, telegram_chat_id_env_ref = $3, slack_webhook_url_env_ref = $4, smtp_host_env_ref = $5, smtp_port_env_ref = $6, smtp_username_env_ref = $7, smtp_password_env_ref = $8, smtp_from_env_ref = $9, smtp_to_env_ref = $10, updated_at = NOW() WHERE organization_id = $11 AND kind = $12`)).WithArgs(true, "NOTIFY_TOKEN", "NOTIFY_CHAT", "", "", "", "", "", "", "", org, "telegram").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, NewRepository(db).UpdateChannel(context.Background(), channel))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM alert_channels WHERE organization_id = $1 AND kind = $2`)).WithArgs(org, "telegram").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, NewRepository(db).DeleteChannel(context.Background(), org, "telegram"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryRejectsUnsupportedChannelAndInvalidOrganizationWithoutDatabaseAccess(t *testing.T) {
	db, mock := thresholdDB(t)
	require.ErrorIs(t, NewRepository(db).CreateChannel(context.Background(), Channel{OrganizationID: "00000000-0000-4000-8000-000000000001", Kind: "webhook"}), ErrInvalidChannel)
	_, err := NewRepository(db).GetChannel(context.Background(), "not-a-uuid", "telegram")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryChannelPropagatesDatabaseErrors(t *testing.T) {
	db, mock := thresholdDB(t)
	org := "00000000-0000-4000-8000-000000000001"
	insertSQL := `INSERT INTO alert_channels (id, organization_id, kind, enabled, telegram_token_env_ref, telegram_chat_id_env_ref, slack_webhook_url_env_ref, smtp_host_env_ref, smtp_port_env_ref, smtp_username_env_ref, smtp_password_env_ref, smtp_from_env_ref, smtp_to_env_ref) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(sqlmock.AnyArg(), org, "telegram", true, "NOTIFY_TOKEN", "NOTIFY_CHAT", "", "", "", "", "", "", "").WillReturnError(sqlmock.ErrCancelled)
	require.ErrorIs(t, NewRepository(db).CreateChannel(context.Background(), testChannel(org)), sqlmock.ErrCancelled)
	require.NoError(t, mock.ExpectationsWereMet())
}
