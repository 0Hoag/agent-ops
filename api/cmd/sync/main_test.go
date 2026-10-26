package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPersistBackfillTwiceUpsertsStableEntitiesAndNormalizedStates(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)

	st := store.New(orm)
	repoID := store.NewUUID()
	mergedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	result := scm.BackfillResult{
		Issues:       []scm.Issue{{Number: 31, Title: "closed issue", State: "closed", Status: scm.StatusDone, Labels: []string{"accepted"}, Assignee: "reviewer", UpdatedAt: updatedAt}},
		PullRequests: []scm.PullRequest{{Number: 32, Title: "merged PR", State: "closed", Merged: true, MergedAt: &mergedAt, HeadSHA: "abc", HeadBranch: "main", UpdatedAt: updatedAt}},
	}
	issueInsert := regexp.MustCompile(`INSERT INTO "issues".*ON CONFLICT \("repository_id","number"\) DO UPDATE SET .*"assignee"=EXCLUDED\.assignee.*"labels"=EXCLUDED\.labels.*"state"=EXCLUDED\.state.*"status"=EXCLUDED\.status.*"title"=EXCLUDED\.title.*"updated_at"=NOW\(\).*"upstream_updated_at"=EXCLUDED\.upstream_updated_at.*WHERE .*upstream_updated_at`)
	pullInsert := regexp.MustCompile(`INSERT INTO "pull_requests".*ON CONFLICT \("repository_id","number"\) DO UPDATE SET .*"branch"=EXCLUDED\.branch.*"closes"=EXCLUDED\.closes.*"head_sha"=EXCLUDED\.head_sha.*"merged_at"=EXCLUDED\.merged_at.*"state"=EXCLUDED\.state.*"title"=EXCLUDED\.title.*"updated_at"=NOW\(\).*"upstream_updated_at"=EXCLUDED\.upstream_updated_at.*WHERE .*upstream_updated_at`)
	pipelineInsert := regexp.MustCompile(`INSERT INTO "pipeline_runs".*ON CONFLICT \("repository_id","external_id"\) DO UPDATE SET .*"conclusion"=EXCLUDED\.conclusion.*"finished_at"=EXCLUDED\.finished_at.*"pr_number"=EXCLUDED\.pr_number.*"sha"=EXCLUDED\.sha.*"started_at"=EXCLUDED\.started_at.*"status"=EXCLUDED\.status.*"updated_at"=NOW\(\).*"upstream_updated_at"=EXCLUDED\.upstream_updated_at.*WHERE .*upstream_updated_at`)
	startedAt := time.Date(2026, 1, 2, 2, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC)
	pipelinePRNumber := 32
	result.PipelineRuns = []scm.PipelineRun{{ExternalID: "run-33", PRNumber: &pipelinePRNumber, SHA: "def", Status: "completed", Conclusion: "success", StartedAt: startedAt, FinishedAt: &finishedAt, UpdatedAt: updatedAt}}
	for pass := 0; pass < 2; pass++ {
		mock.ExpectBegin()
		mock.ExpectExec(issueInsert.String()).WithArgs(sqlmock.AnyArg(), repoID, int64(31), "closed issue", "closed", "done", []byte(`["accepted"]`), "reviewer", updatedAt, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		mock.ExpectBegin()
		mock.ExpectExec(pullInsert.String()).WithArgs(sqlmock.AnyArg(), repoID, int64(32), "merged PR", "closed", "abc", "main", []byte("null"), mergedAt, updatedAt, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		mock.ExpectBegin()
		mock.ExpectExec(pipelineInsert.String()).WithArgs(sqlmock.AnyArg(), repoID, "run-33", int64(32), "def", "completed", "success", startedAt, finishedAt, updatedAt, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		require.NoError(t, persistBackfill(context.Background(), st, repoID, result))
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConfiguredRepositoriesSelectProvidersCredentialsAndIsolatedPersistence(t *testing.T) {
	type observedRequest struct{ uri, credential string }
	var requests []observedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := r.Header.Get("Authorization")
		if credential == "" {
			credential = r.Header.Get("PRIVATE-TOKEN")
		}
		requests = append(requests, observedRequest{r.RequestURI, credential})
		switch {
		case strings.HasPrefix(r.URL.Path, "/gh/repos/acme/one/"):
			require.Equal(t, "Bearer gh-one", credential)
		case strings.HasPrefix(r.URL.Path, "/gl/api/v4/projects/group%2Ftwo/") || strings.HasPrefix(r.URL.EscapedPath(), "/gl/api/v4/projects/group%2Ftwo/"):
			require.Equal(t, "gl-two", credential)
		default:
			t.Errorf("unexpected provider request: %s", r.RequestURI)
		}
		if strings.HasPrefix(r.URL.Path, "/gh/") {
			require.Equal(t, "Bearer gh-one", credential, "GitHub credential resolution")
		}
		if strings.HasPrefix(r.URL.Path, "/gl/") || strings.HasPrefix(r.URL.EscapedPath(), "/gl/") {
			require.Equal(t, "gl-two", credential, "GitLab credential resolution")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/issues") && strings.Contains(r.URL.Path, "/repos/"):
			_, _ = w.Write([]byte(`[{"number":7,"title":"GitHub issue","state":"open","updated_at":"2026-01-02T03:00:00Z","labels":[]}]`))
		case strings.HasSuffix(r.URL.Path, "/issues"):
			_, _ = w.Write([]byte(`[{"iid":7,"title":"GitLab issue","state":"opened","updated_at":"2026-01-02T03:00:00Z","labels":[]}]`))
		default:
			if strings.HasSuffix(r.URL.Path, "/actions/runs") {
				_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		}
	}))
	defer server.Close()

	var cfg config.Config
	raw := fmt.Sprintf(`[{"provider":"github","full_name":"acme/one","token_env":"GH_ONE_TOKEN","webhook_secret_env":"GH_ONE_SECRET","base_url_env":"GH_ONE_URL"},{"provider":"gitlab","full_name":"group/two","token_env":"GL_TWO_TOKEN","webhook_secret_env":"GL_TWO_SECRET","base_url_env":"GL_TWO_URL"}]`)
	require.NoError(t, json.Unmarshal([]byte(raw), &cfg.Repositories))
	t.Setenv("GH_ONE_TOKEN", "gh-one")
	t.Setenv("GH_ONE_SECRET", "gh-secret")
	t.Setenv("GH_ONE_URL", server.URL+"/gh")
	t.Setenv("GL_TWO_TOKEN", "gl-two")
	t.Setenv("GL_TWO_SECRET", "gl-secret")
	t.Setenv("GL_TWO_URL", server.URL+"/gl")

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	st := store.New(orm)
	ids := []store.UUID{store.NewUUID(), store.NewUUID()}
	query := regexp.MustCompile(`INSERT INTO "issues".*ON CONFLICT \("repository_id","number"\) DO UPDATE SET`)
	for pass := 0; pass < 2; pass++ {
		for idx, repository := range cfg.Repositories {
			mock.ExpectQuery(regexp.MustCompile(`SELECT \* FROM "repositories"`).String()).WithArgs(repository.Provider, repository.FullName, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "organization_id", "status_mappings"}).AddRow(ids[idx], repository.Provider, repository.FullName, repository.WebhookSecretEnv, "org-id", []byte(`{}`)))
			mock.ExpectQuery(regexp.MustCompile(`SELECT \* FROM "repositories"`).String()).WithArgs(repository.Provider, repository.FullName, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "full_name", "webhook_secret_env_ref", "organization_id", "status_mappings"}).AddRow(ids[idx], repository.Provider, repository.FullName, repository.WebhookSecretEnv, "org-id", []byte(`{}`)))
			mock.ExpectBegin()
			mock.ExpectExec(query.String()).WithArgs(sqlmock.AnyArg(), ids[idx], int64(7), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`[]`), "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
		}
		require.NoError(t, syncConfiguredRepositories(context.Background(), cfg.Repositories, st))
	}
	require.Len(t, requests, 12)
	require.Contains(t, requests[0].uri, "/gh/repos/acme/one/issues?per_page=100&page=1&state=all")
	require.Contains(t, requests[3].uri, "/gl/api/v4/projects/group%2Ftwo/issues?per_page=100&page=1&state=all")
	require.Equal(t, "Bearer gh-one", requests[0].credential)
	require.Equal(t, "gl-two", requests[3].credential)
	require.Contains(t, requests[6].uri, "/gh/repos/acme/one/issues?per_page=100&page=1&state=all")
	require.Contains(t, requests[9].uri, "/gl/api/v4/projects/group%2Ftwo/issues?per_page=100&page=1&state=all")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryBindingControlRejectsCrossRepositoryWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	st := store.New(orm)
	expectedRepository, wrongRepository := store.NewUUID(), store.NewUUID()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.MustCompile(`INSERT INTO "issues".*ON CONFLICT \("repository_id","number"\) DO UPDATE SET`).String()).WithArgs(sqlmock.AnyArg(), expectedRepository, int64(7), "issue", "open", "todo", []byte(`[]`), "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	err = persistBackfill(context.Background(), st, wrongRepository, scm.BackfillResult{Issues: []scm.Issue{{Number: 7, Title: "issue", State: "open", Status: scm.StatusTodo}}})
	require.ErrorContains(t, err, "repository_id")
	t.Logf("negative control rejected wrong repository binding %s (expected %s): %v", wrongRepository, expectedRepository, err)
}
