package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/gitlab"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	if len(cfg.Repositories) == 0 {
		fail(fmt.Errorf("configure at least one repository"))
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	defer database.Close(db)
	if err = database.RunMigrations(ctx, db); err != nil {
		fail(err)
	}
	st := store.New(db)
	if err = syncConfiguredRepositoriesWithKey(ctx, cfg.Repositories, st, cfg.EncryptionKey); err != nil {
		fail(err)
	}
}

func syncConfiguredRepositories(ctx context.Context, repositories []config.RepositoryConfig, st *store.Store) error {
	return syncConfiguredRepositoriesWithKey(ctx, repositories, st, nil)
}
func syncConfiguredRepositoriesWithKey(ctx context.Context, repositories []config.RepositoryConfig, st *store.Store, key []byte) error {
	var sealer *appcrypto.Sealer
	if len(key) > 0 {
		sealer, _ = appcrypto.New(key)
	}
	return forEachConfiguredRepository(repositories, func(c config.RepositoryConfig) error {
		token := os.Getenv(c.TokenEnv)
		secret := os.Getenv(c.WebhookSecretEnv)
		if err := st.Repositories.Upsert(ctx, &store.Repository{ID: store.NewUUID(), Provider: c.Provider, FullName: c.FullName, WebhookSecretEnvRef: c.WebhookSecretEnv}); err != nil {
			return err
		}
		repo, getErr := st.Repositories.GetByName(ctx, c.Provider, c.FullName)
		if getErr != nil {
			return getErr
		}
		if sealer != nil {
			if value, err := sealer.Open(repo.BackfillTokenCiphertext); err == nil && value != "" {
				token = value
			}
			if value, err := sealer.Open(repo.WebhookSecretCiphertext); err == nil && value != "" {
				secret = value
			}
		}
		baseURL := os.Getenv(c.BaseURLEnv)
		if repo.GitLabBaseURL != "" {
			baseURL = repo.GitLabBaseURL
		}
		p, buildErr := builder.Build(c.Provider, scmConfig(token, secret, baseURL))
		if buildErr != nil {
			return buildErr
		}
		result, backfillErr := p.Backfill(ctx, c.FullName)
		if backfillErr != nil {
			return backfillErr
		}
		var mappings map[string]string
		_ = json.Unmarshal(repo.StatusMappings, &mappings)
		scm.ApplyStatusMapping(c.Provider, &result, mappings)
		if err := persistBackfill(ctx, st, repo.ID, result); err != nil {
			return err
		}
		return nil
	})
}

func forEachConfiguredRepository(repositories []config.RepositoryConfig, syncRepository func(config.RepositoryConfig) error) error {
	for _, repository := range repositories {
		if err := syncRepository(repository); err != nil {
			return err
		}
	}
	return nil
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
	return &store.Issue{ID: store.NewUUID(), RepositoryID: id, Number: int64(i.Number), Title: i.Title, State: i.State, Status: string(i.Status), Labels: store.JSONB(b), Assignee: i.Assignee, UpstreamUpdatedAt: upstreamTime(i.UpdatedAt), UpdatedAt: i.UpdatedAt}
}
func pull(id store.UUID, p scm.PullRequest) *store.PullRequest {
	b, _ := json.Marshal(p.ClosingIssueNumbers)
	return &store.PullRequest{ID: store.NewUUID(), RepositoryID: id, Number: int64(p.Number), Title: p.Title, State: p.State, HeadSHA: p.HeadSHA, Branch: p.HeadBranch, Closes: store.JSONB(b), MergedAt: p.MergedAt, UpstreamUpdatedAt: upstreamTime(p.UpdatedAt), UpdatedAt: p.UpdatedAt}
}
func run(id store.UUID, p scm.PipelineRun) *store.PipelineRun {
	var n *int64
	if p.PRNumber != nil {
		v := int64(*p.PRNumber)
		n = &v
	}
	return &store.PipelineRun{ID: store.NewUUID(), RepositoryID: id, ExternalID: p.ExternalID, PRNumber: n, SHA: p.SHA, Status: p.Status, Conclusion: p.Conclusion, StartedAt: &p.StartedAt, FinishedAt: p.FinishedAt, UpstreamUpdatedAt: upstreamTime(p.UpdatedAt)}
}

func upstreamTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
