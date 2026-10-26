package store

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepositoryCreateGetListAndUniqueUpsert(t *testing.T) {
	store, mock := mockStore(t)
	id := UUID("11111111-1111-1111-1111-111111111111")
	item := &Repository{
		ID: id, Provider: "github", FullName: "acme/app", WebhookSecretEnvRef: "APP_SECRET",
		OrganizationID:          "22222222-2222-2222-2222-222222222222",
		WebhookSecretCiphertext: "sealed-webhook", BackfillTokenCiphertext: "sealed-backfill",
		GitLabBaseURL: "https://gitlab.example", StatusMappings: JSONB(`{"status:review":"review"}`),
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "repositories"`)).WithArgs(id, "github", "acme/app", "APP_SECRET", item.OrganizationID, "sealed-webhook", "sealed-backfill", "https://gitlab.example", sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`{"status:review":"review"}`)).WillReturnRows(sqlmock.NewRows([]string{"status_mappings"}).AddRow([]byte(`{"status:review":"review"}`)))
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
	mock.ExpectQuery(`INSERT INTO "repositories".*ON CONFLICT \("provider","full_name"\).*DO UPDATE SET.*webhook_secret_env_ref.*EXCLUDED\.webhook_secret_env_ref`).WithArgs(id, "github", "acme/app", "APP_SECRET", item.OrganizationID, "sealed-webhook", "sealed-backfill", "https://gitlab.example", sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`{"status:review":"review"}`)).WillReturnRows(sqlmock.NewRows([]string{"status_mappings"}).AddRow([]byte(`{"status:review":"review"}`)))
	mock.ExpectCommit()
	require.NoError(t, store.Repositories.Upsert(context.Background(), item))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryUpsertConfiguredPreservesExistingTenant(t *testing.T) {
	store, mock := mockStore(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).
		WithArgs("gitlab", "tenant/project", 1).
		WillReturnRows(repositoryRows(&Repository{ID: "repo-id", Provider: "gitlab", FullName: "tenant/project", OrganizationID: "org-owned", BackfillTokenCiphertext: "sealed", GitLabBaseURL: "https://gitlab.example"}))
	require.NoError(t, store.Repositories.UpsertConfigured(context.Background(), &Repository{ID: "new-id", Provider: "gitlab", FullName: "tenant/project", OrganizationID: ""}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryUpsertConfiguredRejectsUnboundNewTenant(t *testing.T) {
	store, mock := mockStore(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).
		WithArgs("gitlab", "new/project", 1).WillReturnError(gorm.ErrRecordNotFound)
	err := store.Repositories.UpsertConfigured(context.Background(), &Repository{ID: "new-id", Provider: "gitlab", FullName: "new/project"})
	require.EqualError(t, err, "configured repository gitlab/new/project requires a valid organization_id for a new row")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryUpsertConfiguredRejectsMalformedTenant(t *testing.T) {
	store, mock := mockStore(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE provider = $1 AND full_name = $2 ORDER BY "repositories"."id" LIMIT $3`)).
		WithArgs("gitlab", "malformed/project", 1).WillReturnError(gorm.ErrRecordNotFound)
	err := store.Repositories.UpsertConfigured(context.Background(), &Repository{ID: "new-id", Provider: "gitlab", FullName: "malformed/project", OrganizationID: "not-a-uuid"})
	require.EqualError(t, err, "configured repository gitlab/malformed/project requires a valid organization_id for a new row")
	require.NoError(t, mock.ExpectationsWereMet())
}

func repositoryRows(item *Repository) *sqlmock.Rows {
	return row("id", item.ID, "provider", item.Provider, "full_name", item.FullName, "webhook_secret_env_ref", item.WebhookSecretEnvRef,
		"organization_id", item.OrganizationID, "webhook_secret_ciphertext", item.WebhookSecretCiphertext,
		"backfill_token_ciphertext", item.BackfillTokenCiphertext, "gitlab_base_url", item.GitLabBaseURL,
		"status_mappings", []byte(item.StatusMappings), "created_at", time.Now(), "updated_at", time.Now())
}
