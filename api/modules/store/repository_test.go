package store

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	postgresDriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func mockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgresDriver.New(postgresDriver.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	return New(db), mock
}

func TestWebhookDeliveryIsIdempotent(t *testing.T) {
	store, mock := mockStore(t)
	event := &WebhookEvent{ID: "11111111-1111-1111-1111-111111111111", RepositoryID: "22222222-2222-2222-2222-222222222222", Provider: "github", EventType: "push", DeliveryID: "delivery-1", Payload: JSONB(`{"ok":true}`), ReceivedAt: time.Now()}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	created, err := store.WebhookEvents.CreateIfNew(context.Background(), event)
	require.NoError(t, err)
	require.True(t, created)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "webhook_events"`)).WillReturnResult(sqlmock.NewResult(1, 0))
	mock.ExpectCommit()
	created, err = store.WebhookEvents.CreateIfNew(context.Background(), event)
	require.NoError(t, err)
	require.False(t, created)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIssueListIsRepositoryScopedAndPaginated(t *testing.T) {
	store, mock := mockStore(t)
	repositoryID := UUID("22222222-2222-2222-2222-222222222222")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "issues" WHERE repository_id = $1`)).
		WithArgs(repositoryID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "issues" WHERE repository_id = $1 ORDER BY number DESC LIMIT $2 OFFSET $3`)).
		WithArgs(repositoryID, 20, 20).WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "number", "title", "state", "status", "labels", "assignee", "created_at", "updated_at"}))
	items, page, err := store.Issues.List(context.Background(), repositoryID, 2, 20)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, Page{Page: 2, PerPage: 20, Total: 21}, page)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestJSONBRejectsInvalidPayload(t *testing.T) {
	_, err := JSONB(`not-json`).Value()
	require.Error(t, err)
}
