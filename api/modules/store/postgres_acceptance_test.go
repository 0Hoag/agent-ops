package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresStaleSnapshotAcceptance(t *testing.T) {
	dsn := os.Getenv("W1D_ACCEPTANCE_DSN")
	if dsn == "" {
		t.Skip("set W1D_ACCEPTANCE_DSN to run disposable PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.RunMigrations(context.Background(), db))
	ctx := context.Background()
	repositoryID := UUID("22222222-2222-2222-2222-222222222222")
	require.NoError(t, db.Exec("INSERT INTO repositories (id, provider, full_name, webhook_secret_env_ref) VALUES (?, 'github', ?, 'TEST_SECRET') ON CONFLICT (id) DO NOTHING", repositoryID, "w1d-acceptance").Error)
	store := New(db)
	newer, older := time.Date(2026, 10, 24, 1, 0, 0, 0, time.UTC), time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	for _, reverse := range []bool{false, true} {
		label := "newer-first"
		if reverse {
			label = "older-first"
		}
		t.Run(label, func(t *testing.T) {
			resetAcceptanceRows(t, db, repositoryID)
			write := func(at time.Time, closed bool) {
				state, status, title := "open", "running", "old snapshot"
				if closed {
					state, status, title = "closed", "completed", "new snapshot"
				}
				issue := &Issue{ID: UUID("33333333-3333-3333-3333-333333333333"), RepositoryID: repositoryID, Number: 1, Title: title, State: state, Status: status, Labels: JSONB(`[]`), UpstreamUpdatedAt: &at}
				pull := &PullRequest{ID: UUID("44444444-4444-4444-4444-444444444444"), RepositoryID: repositoryID, Number: 2, Title: title, State: state, HeadSHA: title, Branch: "main", Closes: JSONB(`[]`), UpstreamUpdatedAt: &at}
				run := &PipelineRun{ID: UUID("55555555-5555-5555-5555-555555555555"), RepositoryID: repositoryID, ExternalID: "3", SHA: title, Status: status, Conclusion: status, UpstreamUpdatedAt: &at}
				require.NoError(t, store.Issues.Upsert(ctx, issue))
				require.NoError(t, store.PullRequests.Upsert(ctx, pull))
				require.NoError(t, store.PipelineRuns.Upsert(ctx, run))
			}
			if reverse {
				write(older, false)
				write(newer, true)
			} else {
				write(newer, true)
				write(older, false)
			}
			var issue Issue
			require.NoError(t, db.Where("repository_id = ? AND number = 1", repositoryID).First(&issue).Error)
			require.Equal(t, "closed", issue.State)
			require.Equal(t, "new snapshot", issue.Title)
			require.True(t, newer.Equal(*issue.UpstreamUpdatedAt))
			var pull PullRequest
			require.NoError(t, db.Where("repository_id = ? AND number = 2", repositoryID).First(&pull).Error)
			require.Equal(t, "closed", pull.State)
			require.Equal(t, "new snapshot", pull.Title)
			require.True(t, newer.Equal(*pull.UpstreamUpdatedAt))
			var run PipelineRun
			require.NoError(t, db.Where("repository_id = ? AND external_id = '3'", repositoryID).First(&run).Error)
			require.Equal(t, "completed", run.Status)
			require.Equal(t, "new snapshot", run.SHA)
			require.True(t, newer.Equal(*run.UpstreamUpdatedAt))
		})
	}
	t.Run("missing timestamps remain duplicate safe", func(t *testing.T) {
		resetAcceptanceRows(t, db, repositoryID)
		issue := &Issue{ID: UUID("33333333-3333-3333-3333-333333333333"), RepositoryID: repositoryID, Number: 1, Title: "first delivery", State: "open", Status: "open", Labels: JSONB(`[]`)}
		require.NoError(t, store.Issues.Upsert(ctx, issue))
		issue.Title = "duplicate delivery"
		require.NoError(t, store.Issues.Upsert(ctx, issue))
		var gotIssue Issue
		require.NoError(t, db.Where("repository_id = ? AND number = 1", repositoryID).First(&gotIssue).Error)
		require.Equal(t, "duplicate delivery", gotIssue.Title)
		require.Nil(t, gotIssue.UpstreamUpdatedAt)
		pull := &PullRequest{ID: UUID("44444444-4444-4444-4444-444444444444"), RepositoryID: repositoryID, Number: 2, Title: "first delivery", State: "open", HeadSHA: "first", Branch: "main", Closes: JSONB(`[]`)}
		require.NoError(t, store.PullRequests.Upsert(ctx, pull))
		pull.Title, pull.HeadSHA = "duplicate delivery", "duplicate"
		require.NoError(t, store.PullRequests.Upsert(ctx, pull))
		var gotPull PullRequest
		require.NoError(t, db.Where("repository_id = ? AND number = 2", repositoryID).First(&gotPull).Error)
		require.Equal(t, "duplicate delivery", gotPull.Title)
		require.Nil(t, gotPull.UpstreamUpdatedAt)
		run := &PipelineRun{ID: UUID("55555555-5555-5555-5555-555555555555"), RepositoryID: repositoryID, ExternalID: "3", SHA: "first", Status: "running", Conclusion: ""}
		require.NoError(t, store.PipelineRuns.Upsert(ctx, run))
		run.SHA = "duplicate"
		require.NoError(t, store.PipelineRuns.Upsert(ctx, run))
		var gotRun PipelineRun
		require.NoError(t, db.Where("repository_id = ? AND external_id = '3'", repositoryID).First(&gotRun).Error)
		require.Equal(t, "duplicate", gotRun.SHA)
		require.Nil(t, gotRun.UpstreamUpdatedAt)
	})
	t.Run("concurrent writers", func(t *testing.T) {
		resetAcceptanceRows(t, db, repositoryID)
		var started = make(chan struct{})
		var done = make(chan error, 2)
		for _, at := range []time.Time{older, newer} {
			at := at
			go func() {
				<-started
				state, title, status := "open", "old snapshot", "running"
				if at.Equal(newer) {
					state, title, status = "closed", "new snapshot", "completed"
				}
				s := New(db)
				i := &Issue{ID: UUID("33333333-3333-3333-3333-333333333333"), RepositoryID: repositoryID, Number: 1, Title: title, State: state, Status: status, Labels: JSONB(`[]`), UpstreamUpdatedAt: &at}
				p := &PullRequest{ID: UUID("44444444-4444-4444-4444-444444444444"), RepositoryID: repositoryID, Number: 2, Title: title, State: state, HeadSHA: title, Branch: "main", Closes: JSONB(`[]`), UpstreamUpdatedAt: &at}
				r := &PipelineRun{ID: UUID("55555555-5555-5555-5555-555555555555"), RepositoryID: repositoryID, ExternalID: "3", SHA: title, Status: status, Conclusion: status, UpstreamUpdatedAt: &at}
				if err := s.Issues.Upsert(ctx, i); err != nil {
					done <- err
					return
				}
				if err := s.PullRequests.Upsert(ctx, p); err != nil {
					done <- err
					return
				}
				done <- s.PipelineRuns.Upsert(ctx, r)
			}()
		}
		close(started)
		require.NoError(t, <-done)
		require.NoError(t, <-done)
		var issue Issue
		require.NoError(t, db.Where("repository_id = ? AND number = 1", repositoryID).First(&issue).Error)
		require.Equal(t, "closed", issue.State)
		require.Equal(t, "new snapshot", issue.Title)
		require.True(t, newer.Equal(*issue.UpstreamUpdatedAt))
		var pull PullRequest
		require.NoError(t, db.Where("repository_id = ? AND number = 2", repositoryID).First(&pull).Error)
		require.Equal(t, "closed", pull.State)
		require.Equal(t, "new snapshot", pull.Title)
		require.True(t, newer.Equal(*pull.UpstreamUpdatedAt))
		var run PipelineRun
		require.NoError(t, db.Where("repository_id = ? AND external_id = '3'", repositoryID).First(&run).Error)
		require.Equal(t, "completed", run.Status)
		require.Equal(t, "new snapshot", run.SHA)
		require.True(t, newer.Equal(*run.UpstreamUpdatedAt))
	})
	t.Run("equal and missing timestamps do not overwrite known version", func(t *testing.T) {
		resetAcceptanceRows(t, db, repositoryID)
		equal := newer
		for _, entity := range []string{"issue", "pull", "pipeline"} {
			t.Run(entity, func(t *testing.T) {
				switch entity {
				case "issue":
					base := &Issue{ID: UUID("33333333-3333-3333-3333-333333333333"), RepositoryID: repositoryID, Number: 1, Title: "new snapshot", State: "closed", Status: "closed", Labels: JSONB(`[]`), UpstreamUpdatedAt: &newer}
					require.NoError(t, store.Issues.Upsert(ctx, base))
					for _, at := range []*time.Time{&equal, nil} {
						candidate := *base
						candidate.Title, candidate.State, candidate.UpstreamUpdatedAt = "old snapshot", "open", at
						require.NoError(t, store.Issues.Upsert(ctx, &candidate))
					}
					var got Issue
					require.NoError(t, db.Where("repository_id = ? AND number = 1", repositoryID).First(&got).Error)
					require.Equal(t, "closed", got.State)
					require.Equal(t, "new snapshot", got.Title)
					require.True(t, newer.Equal(*got.UpstreamUpdatedAt))
				case "pull":
					base := &PullRequest{ID: UUID("44444444-4444-4444-4444-444444444444"), RepositoryID: repositoryID, Number: 2, Title: "new snapshot", State: "closed", HeadSHA: "new", Branch: "main", Closes: JSONB(`[]`), UpstreamUpdatedAt: &newer}
					require.NoError(t, store.PullRequests.Upsert(ctx, base))
					for _, at := range []*time.Time{&equal, nil} {
						candidate := *base
						candidate.Title, candidate.State, candidate.UpstreamUpdatedAt = "old snapshot", "open", at
						require.NoError(t, store.PullRequests.Upsert(ctx, &candidate))
					}
					var got PullRequest
					require.NoError(t, db.Where("repository_id = ? AND number = 2", repositoryID).First(&got).Error)
					require.Equal(t, "closed", got.State)
					require.Equal(t, "new snapshot", got.Title)
					require.True(t, newer.Equal(*got.UpstreamUpdatedAt))
				case "pipeline":
					base := &PipelineRun{ID: UUID("55555555-5555-5555-5555-555555555555"), RepositoryID: repositoryID, ExternalID: "3", SHA: "new", Status: "completed", Conclusion: "success", StartedAt: &newer, UpstreamUpdatedAt: &newer}
					require.NoError(t, store.PipelineRuns.Upsert(ctx, base))
					for _, at := range []*time.Time{&equal, nil} {
						candidate := *base
						candidate.SHA, candidate.Status, candidate.UpstreamUpdatedAt = "old", "running", at
						require.NoError(t, store.PipelineRuns.Upsert(ctx, &candidate))
					}
					var got PipelineRun
					require.NoError(t, db.Where("repository_id = ? AND external_id = '3'", repositoryID).First(&got).Error)
					require.Equal(t, "completed", got.Status)
					require.Equal(t, "new", got.SHA)
					require.True(t, newer.Equal(*got.UpstreamUpdatedAt))
				}
			})
		}
	})
}

func resetAcceptanceRows(t *testing.T, db *gorm.DB, repositoryID UUID) {
	t.Helper()
	for _, table := range []string{"issues", "pull_requests", "pipeline_runs"} {
		require.NoError(t, db.Exec(fmt.Sprintf("DELETE FROM %s WHERE repository_id = ?", table), repositoryID).Error)
	}
}
