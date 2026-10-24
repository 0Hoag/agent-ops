package store

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRepositoryCreateGetListAndUniqueUpsert(t *testing.T) {
	store, mock := mockStore(t)
	id := UUID("11111111-1111-1111-1111-111111111111")
	item := &Repository{ID: id, Provider: "github", FullName: "acme/app", WebhookSecretEnvRef: "APP_SECRET"}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "repositories"`)).WithArgs(id, "github", "acme/app", "APP_SECRET", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmockResult())
	mock.ExpectCommit()
	// The helper values are deliberately checked by the SQL driver, including conflict identity.
	require.NoError(t, store.Repositories.Create(context.Background(), item))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE id = $1 ORDER BY "repositories"."id" LIMIT $2`)).WithArgs(id, 1).WillReturnRows(repositoryRows(item))
	got, err := store.Repositories.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, item.FullName, got.FullName)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "repositories"`)).WillReturnRows(rows("count", 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" ORDER BY full_name ASC LIMIT $1`)).WithArgs(20).WillReturnRows(repositoryRows(item))
	items, page, err := store.Repositories.List(context.Background(), 1, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(1), page.Total)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "repositories".*ON CONFLICT \("provider","full_name"\).*DO UPDATE SET.*webhook_secret_env_ref.*EXCLUDED\.webhook_secret_env_ref`).WillReturnResult(sqlmockResult())
	mock.ExpectCommit()
	require.NoError(t, store.Repositories.Upsert(context.Background(), item))
	require.NoError(t, mock.ExpectationsWereMet())
}

func repositoryRows(item *Repository) *sqlmock.Rows {
	return row("id", item.ID, "provider", item.Provider, "full_name", item.FullName, "webhook_secret_env_ref", item.WebhookSecretEnvRef, "created_at", time.Now(), "updated_at", time.Now())
}
