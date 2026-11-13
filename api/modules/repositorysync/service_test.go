package repositorysync

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const exposedSecrets = "Bearer bearer-test-secret https://url-user:url-password@example.test hook=webhook-test-secret"

type backfillProvider struct {
	err    error
	result scm.BackfillResult
}

func (p backfillProvider) VerifyWebhook(context.Context, http.Header, []byte) error { return nil }
func (p backfillProvider) ParseEvent(context.Context, string, http.Header, []byte) (scm.Event, error) {
	return scm.Event{}, nil
}
func (p backfillProvider) Backfill(context.Context, string) (scm.BackfillResult, error) {
	return p.result, p.err
}
func (p backfillProvider) TestRepository(context.Context, string) (string, error) { return "", nil }

func TestSyncFailureStoresAndReturnsOnlySafeError(t *testing.T) {
	service, mock, repository := syncFixture(t, errors.New(exposedSecrets))
	expectRepositoryGet(mock, repository)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "repositories"`).
		WithArgs("provider sync failed", "error", nil, sqlmock.AnyArg(), repository.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := service.Sync(context.Background(), repository.ID)
	require.EqualError(t, err, "provider sync failed")
	require.NotContains(t, err.Error(), "bearer-test-secret")
	require.NotContains(t, err.Error(), "url-password")
	require.NotContains(t, err.Error(), "webhook-test-secret")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncSuccessPersistsSuccessfulStatus(t *testing.T) {
	service, mock, repository := syncFixture(t, nil)
	expectRepositoryGet(mock, repository)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "repositories"`).
		WithArgs("", "success", sqlmock.AnyArg(), sqlmock.AnyArg(), repository.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, service.Sync(context.Background(), repository.ID))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncPersistsPipelineRunsWithProviderIdentity(t *testing.T) {
	service, mock, repository := syncFixture(t, nil)
	service.providers = builder.NewRegistry()
	runAt := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	prNumber := 17
	run := scm.PipelineRun{ExternalID: "run-42", PRNumber: &prNumber, SHA: "sha-42", Status: "completed", Conclusion: "success", StartedAt: runAt, FinishedAt: &runAt, UpdatedAt: runAt}
	require.NoError(t, service.providers.Register("test", func(scm.Config) scm.SCMProvider {
		return backfillProvider{result: scm.BackfillResult{PipelineRuns: []scm.PipelineRun{run}}}
	}))
	expectRepositoryGet(mock, repository)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "pipeline_runs"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "repositories"`).WithArgs("", "success", sqlmock.AnyArg(), sqlmock.AnyArg(), repository.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, service.Sync(context.Background(), repository.ID))
	require.NoError(t, mock.ExpectationsWereMet())
}

func syncFixture(t *testing.T, providerError error) (*Service, sqlmock.Sqlmock, *store.Repository) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sealer, err := appcrypto.New([]byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	token, err := sealer.Seal("bearer-test-secret")
	require.NoError(t, err)
	secret, err := sealer.Seal("webhook-test-secret")
	require.NoError(t, err)
	repository := &store.Repository{ID: "11111111-1111-1111-1111-111111111111", OrganizationID: "22222222-2222-2222-2222-222222222222", Provider: "test", FullName: "acme/app", BackfillTokenCiphertext: token, WebhookSecretCiphertext: secret, GitLabBaseURL: "https://url-user:url-password@example.test"}
	providers := builder.NewRegistry()
	require.NoError(t, providers.Register("test", func(config scm.Config) scm.SCMProvider {
		require.Equal(t, "bearer-test-secret", config.Token)
		require.Equal(t, "webhook-test-secret", config.WebhookSecret)
		require.Equal(t, repository.GitLabBaseURL, config.APIBaseURL)
		return backfillProvider{err: providerError}
	}))
	return NewWithRegistry(store.New(db), sealer, providers), mock, repository
}

func expectRepositoryGet(mock sqlmock.Sqlmock, repository *store.Repository) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "repositories" WHERE id = $1 ORDER BY "repositories"."id" LIMIT $2`)).
		WithArgs(repository.ID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "provider", "full_name", "backfill_token_ciphertext", "webhook_secret_ciphertext", "git_lab_base_url", "status_mappings", "created_at", "updated_at"}).
			AddRow(repository.ID, repository.OrganizationID, repository.Provider, repository.FullName, repository.BackfillTokenCiphertext, repository.WebhookSecretCiphertext, repository.GitLabBaseURL, []byte(`{}`), time.Now(), time.Now()))
}
