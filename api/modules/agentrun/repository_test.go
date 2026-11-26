package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresAgentRunRepository(t *testing.T) {
	dsn := os.Getenv("W3A_AGENT_RUN_ACCEPTANCE_DSN")
	if dsn == "" {
		t.Skip("set W3A_AGENT_RUN_ACCEPTANCE_DSN to use disposable PostgreSQL")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.RunMigrations(context.Background(), db))
	ctx := context.Background()
	organizationID := store.NewUUID()
	require.NoError(t, db.Exec("INSERT INTO organizations (id,name) VALUES (?,?)", organizationID, "agent-run-fixture-"+string(organizationID)).Error)
	ids := []store.UUID{"a3000000-0000-0000-0000-000000000003", "a4000000-0000-0000-0000-000000000004"}
	for i, id := range ids {
		require.NoError(t, db.Exec("INSERT INTO repositories (id, organization_id, provider, full_name, webhook_secret_env_ref) VALUES (?, ?, 'github', ?, 'FIXTURE') ON CONFLICT (id) DO UPDATE SET organization_id=EXCLUDED.organization_id", id, organizationID, []string{"fixture/run-a", "fixture/run-b"}[i]).Error)
	}
	t.Cleanup(func() {
		_ = db.Where("repository_id IN ?", ids).Delete(&AgentRun{}).Error
		_ = db.Where("id IN ?", ids).Delete(&store.Repository{}).Error
		_ = db.Exec("DELETE FROM organizations WHERE id = ?", organizationID).Error
	})
	repo := NewRepository(db)
	runs := []AgentRun{
		{ID: store.NewUUID(), RepositoryID: uuidPtr(ids[0]), Wave: "w3a", Worker: "alpha", Model: "known", IssueNumbers: store.JSONB(`[24,25]`), InputTokens: 10},
		{ID: store.NewUUID(), RepositoryID: uuidPtr(ids[0]), Wave: "w3b", Worker: "beta", Model: "known", IssueNumbers: store.JSONB(`[25]`), InputTokens: 20},
		{ID: store.NewUUID(), RepositoryID: uuidPtr(ids[1]), Wave: "w3a", Worker: "alpha", Model: "known", IssueNumbers: store.JSONB(`[24]`), InputTokens: 30},
	}
	for i := range runs {
		run := &runs[i]
		run.StartedAt = time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC)
		run.EndedAt = run.StartedAt
		require.NoError(t, repo.Create(ctx, run))
	}
	all, err := repo.List(ctx, Filter{RepositoryID: string(ids[0])})
	require.NoError(t, err)
	require.Len(t, all, 2)
	byWave, err := repo.List(ctx, Filter{RepositoryID: string(ids[0]), Wave: "w3a"})
	require.NoError(t, err)
	require.Len(t, byWave, 1)
	require.Equal(t, int64(10), byWave[0].InputTokens)
	byIssue, err := repo.List(ctx, Filter{RepositoryID: string(ids[0]), IssueNumber: 24})
	require.NoError(t, err)
	require.Len(t, byIssue, 1)
	require.Equal(t, "w3a", byIssue[0].Wave)
	groups, err := repo.Aggregate(ctx, Filter{RepositoryID: string(ids[0])}, map[string]Price{"known": {InputPerMillion: 1}})
	require.NoError(t, err)
	require.Len(t, groups, 3)
	for _, group := range groups {
		if group.Wave == "w3a" && group.Worker == "alpha" && *group.IssueNumber == 24 {
			require.NotNil(t, group.IssueNumber)
			require.EqualValues(t, 24, *group.IssueNumber)
			require.EqualValues(t, 10, group.InputTokens)
			require.NotNil(t, group.CostUSD)
		}
	}
	encoded, err := json.Marshal(groups)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "w3a")

	localSession := "home-" + string(store.NewUUID())
	localID := store.NewUUID()
	otherOrganizationID := store.NewUUID()
	require.NoError(t, db.Exec("INSERT INTO organizations (id,name) VALUES (?,?)", otherOrganizationID, "local-test-"+string(otherOrganizationID)).Error)

	local := &AgentRun{ID: localID, OrganizationID: organizationID, Wave: "local", Worker: "claude", Model: "claude-sonnet", IssueNumbers: store.JSONB(`[]`), StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC(), Harness: strPtr("claude"), SessionID: &localSession, InputTokens: 10}
	require.NoError(t, repo.Create(ctx, local))
	firstID := local.ID
	local.InputTokens = 25
	require.NoError(t, repo.Create(ctx, local))
	require.Equal(t, firstID, local.ID, "repeated local session updates the existing run")
	orgRuns, err := repo.List(WithOrganization(ctx, string(organizationID)), Filter{})
	require.NoError(t, err)
	var localRuns []AgentRun
	for _, run := range orgRuns {
		if run.SessionID != nil && *run.SessionID == localSession {
			localRuns = append(localRuns, run)
		}
	}
	require.Len(t, localRuns, 1)
	require.EqualValues(t, 25, localRuns[0].InputTokens)
	require.Nil(t, localRuns[0].RepositoryID)
	require.Nil(t, localRuns[0].PRNumber)
	page, _, err := repo.ListPage(WithOrganization(ctx, string(organizationID)), Filter{Harness: "claude"}, 1, 20)
	require.NoError(t, err)
	for _, run := range page {
		if run.ID == firstID {
			require.NotNil(t, run.Relations)
			require.Empty(t, run.Relations.PullRequests)
			require.Empty(t, run.Relations.Issues)
			require.Empty(t, run.Relations.CI)
		}
	}
	repoRuns, err := repo.List(WithOrganization(ctx, string(organizationID)), Filter{RepositoryID: string(ids[0])})
	require.NoError(t, err)
	for _, run := range repoRuns {
		require.NotEqual(t, firstID, run.ID)
	}
	otherLocal := &AgentRun{ID: store.NewUUID(), OrganizationID: otherOrganizationID, Wave: "local", Worker: "claude", Model: "claude-sonnet", IssueNumbers: store.JSONB(`[]`), StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC(), Harness: strPtr("claude"), SessionID: strPtr("other-session"), InputTokens: 2}
	require.NoError(t, repo.Create(ctx, otherLocal))
	otherOrgRuns, err := repo.List(WithOrganization(ctx, string(otherOrganizationID)), Filter{})
	require.NoError(t, err)
	require.Len(t, otherOrgRuns, 1)
	require.Equal(t, otherLocal.ID, otherOrgRuns[0].ID)
	require.NotEqual(t, firstID, otherOrgRuns[0].ID)
	t.Cleanup(func() {
		_ = db.Where("id IN ?", []store.UUID{localID, otherLocal.ID}).Delete(&AgentRun{}).Error
		_ = db.Exec("DELETE FROM organizations WHERE id = ?", otherOrganizationID).Error
	})
}

func TestAggregateUnknownCostIsOrderIndependent(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	repo := NewRepository(db)
	knownPrice := map[string]Price{"known": {InputPerMillion: 1}}
	knownPrice["zero"] = Price{}
	for _, tc := range []struct {
		name   string
		models []string
		known  bool
		cost   float64
	}{
		{name: "known unknown known", models: []string{"known", "unknown", "known"}},
		{name: "unknown known", models: []string{"unknown", "known"}},
		{name: "known only", models: []string{"known", "known"}, known: true, cost: 2e-6},
		{name: "known zero", models: []string{"zero"}, known: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := sqlmock.NewRows([]string{"id", "repository_id", "wave", "worker", "model", "issue_numbers", "input_tokens", "cached_input_tokens", "cache_write_tokens", "output_tokens", "reasoning_tokens", "cost_usd", "started_at", "ended_at", "created_at", "updated_at"})
			for i, model := range tc.models {
				tokens := int64(1)
				if tc.name == "known zero" {
					tokens = 0
				}
				rows.AddRow(store.NewUUID(), nil, "w", "worker", model, []byte(`[1]`), tokens, 0, 0, 0, 0, nil, time.Date(2026, 1, len(tc.models)-i, 0, 0, 0, 0, time.UTC), nil, nil, nil)
			}
			mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_runs" ORDER BY started_at DESC`)).WillReturnRows(rows)
			groups, err := repo.Aggregate(context.Background(), Filter{}, knownPrice)
			require.NoError(t, err)
			require.Len(t, groups, 1)
			if tc.known {
				require.NotNil(t, groups[0].CostUSD)
				require.InDelta(t, tc.cost, *groups[0].CostUSD, 1e-12)
			} else {
				require.Nil(t, groups[0].CostUSD)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPRNumberByBranchOnlyLinksOneOpenRequestInRepository(t *testing.T) {
	repositoryID := "repo-a"
	branch := "reviews/development"
	for _, tc := range []struct {
		name string
		rows *sqlmock.Rows
		want *int64
	}{
		{
			name: "closed-only candidate is ignored",
			// The repository has an exact-branch MR, but its stored state is merged.
			// The resolver must query only open MRs, so that candidate cannot link.
			rows: sqlmock.NewRows([]string{"number"}),
		},
		{
			name: "one open match is linked",
			rows: sqlmock.NewRows([]string{"number"}).AddRow(int64(109)),
			want: ptr(int64(109)),
		},
		{
			name: "ambiguous open matches are not linked",
			rows: sqlmock.NewRows([]string{"number"}).AddRow(int64(112)).AddRow(int64(111)),
		},
		{
			name: "matching branch in another repository is not linked",
			rows: sqlmock.NewRows([]string{"number"}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
			require.NoError(t, err)

			mock.ExpectQuery(`SELECT .* FROM "pull_requests" WHERE repository_id = \$1 AND branch = \$2 AND state = \$3 ORDER BY number DESC LIMIT \$4`).
				WithArgs(repositoryID, branch, "open", 2).
				WillReturnRows(tc.rows)

			got, err := NewRepository(db).PRNumberByBranch(context.Background(), repositoryID, branch)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func ptr(value int64) *int64 { return &value }

func uuidPtr(id store.UUID) *store.UUID { return &id }

func TestOrgScopedListIncludesLocalRunsAndRepositoryFilterIsExplicit(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	columns := []string{"id", "repository_id", "organization_id", "wave", "worker", "model", "issue_numbers", "started_at", "ended_at"}
	mock.ExpectQuery(`SELECT .* FROM "agent_runs" WHERE agent_runs.organization_id = \$1 ORDER BY started_at DESC`).WithArgs("org-a").WillReturnRows(sqlmock.NewRows(columns).AddRow("run", nil, "org-a", "local", "claude", "sonnet", []byte("[]"), time.Now(), time.Now()))
	runs, err := NewRepository(db).List(WithOrganization(context.Background(), "org-a"), Filter{})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Nil(t, runs[0].RepositoryID)
	mock.ExpectQuery(`SELECT .* FROM "agent_runs" WHERE agent_runs.organization_id = \$1 AND repository_id = \$2 ORDER BY started_at DESC`).WithArgs("org-a", "repo-a").WillReturnRows(sqlmock.NewRows(columns))
	runs, err = NewRepository(db).List(WithOrganization(context.Background(), "org-a"), Filter{RepositoryID: "repo-a"})
	require.NoError(t, err)
	require.Empty(t, runs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRejectsRepositoryOutsideConfiguredOrganization(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT .*organization_id.*FROM "repositories" WHERE id = \$1`).WithArgs("repo-a").WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow("org-b"))
	repoID := store.UUID("repo-a")
	run := &AgentRun{ID: store.NewUUID(), RepositoryID: &repoID, OrganizationID: store.UUID("org-a")}
	err = NewRepository(db).Create(context.Background(), run)
	require.ErrorContains(t, err, "does not match configured organization")
	require.NoError(t, mock.ExpectationsWereMet())
}

func strPtr(value string) *string { return &value }
