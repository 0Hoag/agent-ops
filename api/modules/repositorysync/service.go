package repositorysync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
)

type Service struct {
	repositories *store.Store
	sealer       *appcrypto.Sealer
	providers    *builder.Registry
	now          func() time.Time
}

func New(st *store.Store, sealer *appcrypto.Sealer) *Service {
	return NewWithRegistry(st, sealer, builder.Default())
}

func NewWithRegistry(st *store.Store, sealer *appcrypto.Sealer, providers *builder.Registry) *Service {
	return &Service{repositories: st, sealer: sealer, providers: providers, now: time.Now}
}

var (
	errLoadRepositories = errors.New("load repositories for sync failed")
	errLoadRepository   = errors.New("load repository for sync failed")
	errProviderSync     = errors.New("provider sync failed")
	errStoreStatus      = errors.New("store repository sync status failed")
)

func (s *Service) Sync(ctx context.Context, id store.UUID) error {
	r, err := s.repositories.Repositories.Get(ctx, id)
	if err != nil {
		return errLoadRepository
	}
	if err = s.syncOne(ctx, r); err != nil {
		if statusErr := s.repositories.Repositories.UpdateSyncStatus(ctx, id, nil, "error", errProviderSync.Error()); statusErr != nil {
			return errStoreStatus
		}
		return errProviderSync
	}
	at := s.now().UTC()
	if err := s.repositories.Repositories.UpdateSyncStatus(ctx, id, &at, "success", ""); err != nil {
		return errStoreStatus
	}
	return nil
}
func (s *Service) SyncAll(ctx context.Context) error {
	rows, err := s.repositories.Repositories.ListAll(ctx)
	if err != nil {
		return errLoadRepositories
	}
	var first error
	for i := range rows {
		if err := s.Sync(ctx, rows[i].ID); err != nil && first == nil {
			first = err
		}
	}
	return first
}
func (s *Service) syncOne(ctx context.Context, r *store.Repository) error {
	token, secret := r.BackfillTokenCiphertext, r.WebhookSecretCiphertext
	if s.sealer == nil {
		return fmt.Errorf("repository encryption is not configured")
	}
	if token, _ = s.sealer.Open(token); token == "" {
		return fmt.Errorf("repository token is not configured")
	}
	if secret, _ = s.sealer.Open(secret); secret == "" {
		return fmt.Errorf("repository webhook secret is not configured")
	}
	p, err := s.providers.Build(r.Provider, scm.Config{Token: token, WebhookSecret: secret, APIBaseURL: r.GitLabBaseURL})
	if err != nil {
		return err
	}
	result, err := p.Backfill(ctx, r.FullName)
	if err != nil {
		return err
	}
	var mappings map[string]string
	_ = json.Unmarshal(r.StatusMappings, &mappings)
	scm.ApplyStatusMapping(r.Provider, &result, mappings)
	for _, i := range result.Issues {
		b, _ := json.Marshal(i.Labels)
		if err := s.repositories.Issues.Upsert(ctx, &store.Issue{ID: store.NewUUID(), RepositoryID: r.ID, Number: int64(i.Number), Title: i.Title, State: i.State, Status: string(i.Status), Labels: store.JSONB(b), Assignee: i.Assignee, UpstreamUpdatedAt: nonzero(i.UpdatedAt), UpdatedAt: i.UpdatedAt}); err != nil {
			return err
		}
	}
	for _, p := range result.PullRequests {
		b, _ := json.Marshal(p.ClosingIssueNumbers)
		if err := s.repositories.PullRequests.Upsert(ctx, &store.PullRequest{ID: store.NewUUID(), RepositoryID: r.ID, Number: int64(p.Number), Title: p.Title, State: p.State, HeadSHA: p.HeadSHA, Branch: p.HeadBranch, Closes: store.JSONB(b), MergedAt: p.MergedAt, UpstreamUpdatedAt: nonzero(p.UpdatedAt), UpdatedAt: p.UpdatedAt}); err != nil {
			return err
		}
	}
	return nil
}
func nonzero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
