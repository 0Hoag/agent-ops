package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"gorm.io/gorm"
	"net/http"
)

type Service struct {
	db           *gorm.DB
	repositories *store.Store
	registry     ProviderRegistry
}
type ProviderRegistry interface {
	Build(string, scm.Config) (scm.SCMProvider, error)
}

func NewService(db *gorm.DB, registry ProviderRegistry) *Service {
	return &Service{db: db, repositories: store.New(db), registry: registry}
}

func (service *Service) Ingest(ctx context.Context, provider, secret, token, baseURL string, h http.Header, payload []byte) (bool, error) {
	providerClient, err := service.registry.Build(provider, scm.Config{WebhookSecret: secret, Token: token, APIBaseURL: baseURL})
	if err != nil {
		return false, fmt.Errorf("unsupported provider")
	}
	if err := providerClient.VerifyWebhook(ctx, h, payload); err != nil {
		return false, fmt.Errorf("invalid webhook signature")
	}
	event, err := providerClient.ParseEvent(ctx, header(h, "X-GitHub-Event"), h, payload)
	if err != nil {
		return false, fmt.Errorf("invalid webhook event")
	}
	if event.Repository == "" || event.DeliveryID == "" {
		return false, fmt.Errorf("webhook metadata is incomplete")
	}
	result := false
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		st := store.New(tx)
		repo, lookupErr := st.Repositories.GetByName(ctx, provider, event.Repository)
		if lookupErr != nil {
			return fmt.Errorf("repository is not configured")
		}
		id := store.NewUUID()
		created, createErr := st.WebhookEvents.CreateIfNew(ctx, &store.WebhookEvent{ID: id, RepositoryID: repo.ID, Provider: provider, EventType: event.EventType, DeliveryID: event.DeliveryID, Payload: store.JSONB(payload), ReceivedAt: event.ReceivedAt})
		if createErr != nil {
			return createErr
		}
		if !created {
			return nil
		}
		result = true
		if event.Issue != nil {
			return st.Issues.Upsert(ctx, issueModel(repo.ID, event.Issue))
		}
		if event.PullRequest != nil {
			return st.PullRequests.Upsert(ctx, pullModel(repo.ID, event.PullRequest))
		}
		if event.PipelineRun != nil {
			return st.PipelineRuns.Upsert(ctx, runModel(repo.ID, event.PipelineRun))
		}
		return nil
	})
	return result, err
}

func header(h map[string][]string, key string) string {
	for k, v := range h {
		if len(v) > 0 && (k == key || (len(k) == len(key) && equalFold(k, key))) {
			return v[0]
		}
	}
	return ""
}
func equalFold(a, b string) bool {
	for i := range a {
		if a[i]|32 != b[i]|32 {
			return false
		}
	}
	return true
}
func issueModel(id store.UUID, i *scm.Issue) *store.Issue {
	labels, _ := json.Marshal(i.Labels)
	return &store.Issue{ID: store.NewUUID(), RepositoryID: id, Number: int64(i.Number), Title: i.Title, State: i.State, Status: string(i.Status), Labels: store.JSONB(labels), Assignee: i.Assignee, UpdatedAt: i.UpdatedAt}
}
func pullModel(id store.UUID, p *scm.PullRequest) *store.PullRequest {
	closes, _ := json.Marshal(p.ClosingIssueNumbers)
	return &store.PullRequest{ID: store.NewUUID(), RepositoryID: id, Number: int64(p.Number), Title: p.Title, State: p.State, HeadSHA: p.HeadSHA, Branch: p.HeadBranch, Closes: store.JSONB(closes), MergedAt: p.MergedAt, UpdatedAt: p.UpdatedAt}
}
func runModel(id store.UUID, p *scm.PipelineRun) *store.PipelineRun {
	var n *int64
	if p.PRNumber != nil {
		v := int64(*p.PRNumber)
		n = &v
	}
	return &store.PipelineRun{ID: store.NewUUID(), RepositoryID: id, ExternalID: p.ExternalID, PRNumber: n, SHA: p.SHA, Status: p.Status, Conclusion: p.Conclusion, StartedAt: &p.StartedAt, FinishedAt: p.FinishedAt}
}
