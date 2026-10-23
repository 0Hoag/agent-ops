package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	if len(cfg.Repositories) != 1 {
		fail(fmt.Errorf("configure exactly one repository"))
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	defer database.Close(db)
	st := store.New(db)
	c := cfg.Repositories[0]
	token := os.Getenv(c.TokenEnv)
	secret := os.Getenv(c.WebhookSecretEnv)
	p, err := builder.Build(c.Provider, scmConfig(token, secret, os.Getenv(c.BaseURLEnv)))
	if err != nil {
		fail(err)
	}
	if err = st.Repositories.Upsert(ctx, &store.Repository{ID: store.NewUUID(), Provider: c.Provider, FullName: c.FullName, WebhookSecretEnvRef: c.WebhookSecretEnv}); err != nil {
		fail(err)
	}
	repo, err := st.Repositories.GetByName(ctx, c.Provider, c.FullName)
	if err != nil {
		fail(err)
	}
	result, err := p.Backfill(ctx, c.FullName)
	if err != nil {
		fail(err)
	}
	if err = persistBackfill(ctx, st, repo.ID, result); err != nil {
		fail(err)
	}
}

func persistBackfill(ctx context.Context, st *store.Store, repositoryID store.UUID, result scm.BackfillResult) error {
	for _, item := range result.Issues {
		if err := st.Issues.Upsert(ctx, issue(repositoryID, item)); err != nil {
			return err
		}
	}
	for _, item := range result.PullRequests {
		if err := st.PullRequests.Upsert(ctx, pull(repositoryID, item)); err != nil {
			return err
		}
	}
	for _, item := range result.PipelineRuns {
		if err := st.PipelineRuns.Upsert(ctx, run(repositoryID, item)); err != nil {
			return err
		}
	}
	return nil
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
func scmConfig(t, s, b string) scm.Config {
	return scm.Config{Token: t, WebhookSecret: s, APIBaseURL: b}
}
func issue(id store.UUID, i scm.Issue) *store.Issue {
	b, _ := json.Marshal(i.Labels)
	return &store.Issue{ID: store.NewUUID(), RepositoryID: id, Number: int64(i.Number), Title: i.Title, State: i.State, Status: string(i.Status), Labels: store.JSONB(b), Assignee: i.Assignee, UpdatedAt: i.UpdatedAt}
}
func pull(id store.UUID, p scm.PullRequest) *store.PullRequest {
	b, _ := json.Marshal(p.ClosingIssueNumbers)
	return &store.PullRequest{ID: store.NewUUID(), RepositoryID: id, Number: int64(p.Number), Title: p.Title, State: p.State, HeadSHA: p.HeadSHA, Branch: p.HeadBranch, Closes: store.JSONB(b), MergedAt: p.MergedAt, UpdatedAt: p.UpdatedAt}
}
func run(id store.UUID, p scm.PipelineRun) *store.PipelineRun {
	var n *int64
	if p.PRNumber != nil {
		v := int64(*p.PRNumber)
		n = &v
	}
	return &store.PipelineRun{ID: store.NewUUID(), RepositoryID: id, ExternalID: p.ExternalID, PRNumber: n, SHA: p.SHA, Status: p.Status, Conclusion: p.Conclusion, StartedAt: &p.StartedAt, FinishedAt: p.FinishedAt}
}
