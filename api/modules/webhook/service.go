package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"gorm.io/gorm"
)

var ErrRejected = errors.New("webhook rejected")

type Service struct {
	db           *gorm.DB
	repositories *store.Store
	registry     ProviderRegistry
	publisher    Publisher
	sealer       *appcrypto.Sealer
	metrics      LagObserver
	now          func() time.Time
	limiter      *RateLimiter
}
type LagObserver interface{ ObserveWebhook(string, time.Time) }

type ProviderRegistry interface {
	Build(string, scm.Config) (scm.SCMProvider, error)
}

func NewService(db *gorm.DB, registry ProviderRegistry) *Service {
	return NewServiceWithPublisher(db, registry, nil)
}

func NewServiceWithPublisher(db *gorm.DB, registry ProviderRegistry, publisher Publisher) *Service {
	return &Service{db: db, repositories: store.New(db), registry: registry, publisher: publisher, now: time.Now, limiter: NewRateLimiter(defaultRateLimit, defaultRateWindow, defaultRateEntries, nil)}
}

func NewServiceWithPublisherAndSealer(db *gorm.DB, registry ProviderRegistry, publisher Publisher, sealer *appcrypto.Sealer) *Service {
	service := NewServiceWithPublisher(db, registry, publisher)
	service.sealer = sealer
	return service
}
func (service *Service) SetMetrics(observer LagObserver, now func() time.Time) {
	service.metrics = observer
	if now != nil {
		service.now = now
	}
}

func (service *Service) Ingest(ctx context.Context, provider, configuredRepository, secret, token, baseURL string, h http.Header, payload []byte) (bool, error) {
	providerClient, err := service.registry.Build(provider, scm.Config{WebhookSecret: secret, Token: token, APIBaseURL: baseURL})
	if err != nil {
		return false, ErrRejected
	}
	if err := providerClient.VerifyWebhook(ctx, h, payload); err != nil {
		return false, ErrRejected
	}
	return service.persistVerified(ctx, provider, configuredRepository, providerClient, h, payload)
}

func (service *Service) VerifyConfigured(ctx context.Context, provider, secret, token, baseURL string, h http.Header, payload []byte) (scm.SCMProvider, error) {
	providerClient, err := service.registry.Build(provider, scm.Config{WebhookSecret: secret, Token: token, APIBaseURL: baseURL})
	if err != nil {
		return nil, ErrRejected
	}
	if err := providerClient.VerifyWebhook(ctx, h, payload); err != nil {
		return nil, ErrRejected
	}
	return providerClient, nil
}

func (service *Service) VerifyConfiguredForRepository(ctx context.Context, provider, repository, secret, token, baseURL string, h http.Header, payload []byte) (scm.SCMProvider, error) {
	if stored, err := service.repositories.Repositories.GetByName(ctx, provider, repository); err == nil {
		if service.sealer != nil {
			if value, openErr := service.sealer.Open(stored.WebhookSecretCiphertext); openErr == nil && value != "" {
				secret = value
			}
			if value, openErr := service.sealer.Open(stored.BackfillTokenCiphertext); openErr == nil && value != "" {
				token = value
			}
		}
		if stored.GitLabBaseURL != "" {
			baseURL = stored.GitLabBaseURL
		}
	}
	return service.VerifyConfigured(ctx, provider, secret, token, baseURL, h, payload)
}

func (service *Service) IngestParsed(ctx context.Context, provider, configuredRepository string, providerClient scm.SCMProvider, h http.Header, payload []byte) (bool, error) {
	event, err := providerClient.ParseEvent(ctx, eventType(provider, h), h, payload)
	if err != nil || !strings.EqualFold(event.Repository, configuredRepository) {
		return false, ErrCandidateMismatch
	}
	if repo, lookupErr := service.repositories.Repositories.GetByName(ctx, provider, configuredRepository); lookupErr == nil && event.Issue != nil {
		var mappings map[string]string
		_ = json.Unmarshal(repo.StatusMappings, &mappings)
		result := scm.BackfillResult{Issues: []scm.Issue{*event.Issue}}
		scm.ApplyStatusMapping(provider, &result, mappings)
		*event.Issue = result.Issues[0]
	}
	return service.persistEvent(ctx, provider, configuredRepository, event, payload)
}

func eventType(provider string, h http.Header) string {
	if strings.EqualFold(provider, "gitlab") {
		return header(h, "X-Gitlab-Event")
	}
	return header(h, "X-GitHub-Event")
}

func (service *Service) persistVerified(ctx context.Context, provider, configuredRepository string, providerClient scm.SCMProvider, h http.Header, payload []byte) (bool, error) {
	event, err := providerClient.ParseEvent(ctx, eventType(provider, h), h, payload)
	if err != nil {
		return false, ErrRejected
	}
	return service.persistEvent(ctx, provider, configuredRepository, event, payload)
}

var ErrCandidateMismatch = errors.New("webhook candidate mismatch")
var ErrRateLimited = errors.New("webhook rate limit exceeded")

func (service *Service) persistEvent(ctx context.Context, provider, configuredRepository string, event scm.Event, payload []byte) (bool, error) {
	started := service.now()
	if !strings.EqualFold(event.Repository, configuredRepository) {
		return false, ErrCandidateMismatch
	}
	if event.DeliveryID == "" || !validEntity(event) {
		return false, ErrRejected
	}
	result := false
	var repositoryID store.UUID
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		st := store.New(tx)
		repo, lookupErr := st.Repositories.GetByName(ctx, provider, configuredRepository)
		if lookupErr != nil {
			return ErrRejected
		}
		if service.limiter != nil && !service.limiter.Allow(repo.ID) {
			return ErrRateLimited
		}
		id := store.NewUUID()
		created, createErr := st.WebhookEvents.CreateIfNew(ctx, &store.WebhookEvent{ID: id, RepositoryID: repo.ID, Provider: provider, EventType: event.EventType, DeliveryID: event.DeliveryID, Payload: store.JSONB(payload), ReceivedAt: event.ReceivedAt})
		if createErr != nil {
			return createErr
		}
		if !created {
			return nil
		}
		repositoryID = repo.ID
		if event.Issue != nil {
			if err := st.Issues.Upsert(ctx, issueModel(repo.ID, event.Issue)); err != nil {
				return err
			}
		}
		if event.PullRequest != nil {
			if err := st.PullRequests.Upsert(ctx, pullModel(repo.ID, event.PullRequest)); err != nil {
				return err
			}
		}
		if event.PipelineRun != nil {
			if err := st.PipelineRuns.Upsert(ctx, runModel(repo.ID, event.PipelineRun)); err != nil {
				return err
			}
		}
		result = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if result && service.publisher != nil {
		service.publisher.Publish(string(repositoryID))
	}
	if result && service.metrics != nil {
		service.metrics.ObserveWebhook(provider, started)
	}
	return result, nil
}

func validEntity(event scm.Event) bool {
	count := 0
	if event.Issue != nil {
		count++
		if event.Issue.Number <= 0 || event.Issue.Title == "" || (event.Issue.State != "open" && event.Issue.State != "closed") {
			return false
		}
	}
	if event.PullRequest != nil {
		count++
		if event.PullRequest.Number <= 0 || event.PullRequest.Title == "" || event.PullRequest.HeadSHA == "" || event.PullRequest.HeadBranch == "" || (event.PullRequest.State != "open" && event.PullRequest.State != "closed" && event.PullRequest.State != "merged") {
			return false
		}
	}
	if event.PipelineRun != nil {
		count++
		if event.PipelineRun.ExternalID == "" || event.PipelineRun.ExternalID == "0" || event.PipelineRun.SHA == "" || event.PipelineRun.Status == "" || event.PipelineRun.StartedAt.IsZero() {
			return false
		}
	}
	return count == 1
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
	return &store.Issue{ID: store.NewUUID(), RepositoryID: id, Number: int64(i.Number), Title: i.Title, State: i.State, Status: string(i.Status), Labels: store.JSONB(labels), Assignee: i.Assignee, UpstreamUpdatedAt: upstreamTime(i.UpdatedAt), UpdatedAt: i.UpdatedAt}
}
func pullModel(id store.UUID, p *scm.PullRequest) *store.PullRequest {
	closes, _ := json.Marshal(p.ClosingIssueNumbers)
	return &store.PullRequest{ID: store.NewUUID(), RepositoryID: id, Number: int64(p.Number), Title: p.Title, State: p.State, HeadSHA: p.HeadSHA, Branch: p.HeadBranch, Closes: store.JSONB(closes), MergedAt: p.MergedAt, UpstreamUpdatedAt: upstreamTime(p.UpdatedAt), UpdatedAt: p.UpdatedAt}
}
func runModel(id store.UUID, p *scm.PipelineRun) *store.PipelineRun {
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
