package alert

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/notification"
	"gorm.io/gorm"
)

const defaultRuntimeInterval = time.Minute

// Runtime polls persisted observations with a bounded per-tick workload.
type Runtime struct {
	db       *gorm.DB
	interval time.Duration
	limit    int
	mu       sync.Mutex
	services map[string]*Service
}

func NewRuntime(db *gorm.DB, interval time.Duration, limit int) *Runtime {
	if interval <= 0 {
		interval = defaultRuntimeInterval
	}
	if limit <= 0 {
		limit = 100
	}
	return &Runtime{db: db, interval: interval, limit: limit, services: make(map[string]*Service)}
}

func (r *Runtime) Start(ctx context.Context) {
	go func() {
		_ = r.Trigger(ctx)
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				_ = r.triggerAt(ctx, now)
			}
		}
	}()
}

func (r *Runtime) Trigger(ctx context.Context) error { return r.triggerAt(ctx, time.Now()) }

func (r *Runtime) triggerAt(ctx context.Context, now time.Time) error {
	var repos []struct{ ID, OrganizationID, FullName string }
	if err := r.db.WithContext(ctx).Table("repositories").Select("id, organization_id, full_name").Order("id").Limit(r.limit).Find(&repos).Error; err != nil {
		return fmt.Errorf("list alert repositories: %w", err)
	}
	for _, repo := range repos {
		if err := ctx.Err(); err != nil {
			return err
		}
		thresholds, err := NewRepository(r.db).GetThresholds(ctx, repo.OrganizationID)
		if err != nil {
			return err
		}
		if thresholds == nil {
			thresholds = &defaultThresholds
		}
		channels, err := r.enabledChannels(ctx, repo.OrganizationID)
		if err != nil {
			return err
		}
		observation, err := r.observation(ctx, repo)
		if err != nil {
			return err
		}
		if err := r.service(repo.OrganizationID, *thresholds, channels).EvaluateAndDeliver(ctx, now, observation); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) service(org string, thresholds Thresholds, channels map[string]notification.Notifier) *Service {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Rebuild on every tick so persisted threshold/channel updates apply to the next observation.
	s := NewService(NewEvaluator(thresholds), NewRepository(r.db), channels)
	r.services[org] = s
	return s
}

func (r *Runtime) observation(ctx context.Context, repo struct{ ID, OrganizationID, FullName string }) (Observation, error) {
	o := Observation{OrganizationID: repo.OrganizationID, Repository: repo.FullName}
	var runs []struct {
		Worker                         string
		StartedAt, UpdatedAt           time.Time
		InputTokens, CachedInputTokens int64
		EndedAt                        *time.Time
	}
	if err := r.db.WithContext(ctx).Table("agent_runs").Select("worker, started_at, updated_at, input_tokens, cached_input_tokens, ended_at").Where("repository_id = ?", repo.ID).Order("started_at DESC").Limit(100).Find(&runs).Error; err != nil {
		return o, err
	}
	for _, v := range runs {
		last := v.UpdatedAt
		if last.IsZero() {
			last = v.StartedAt
		}
		o.Runs = append(o.Runs, RunObservation{Worker: v.Worker, StartedAt: v.StartedAt, LastActivityAt: last, InputTokens: v.InputTokens, CachedInputTokens: v.CachedInputTokens, Active: v.EndedAt == nil})
	}
	var pipelines []struct{ ID, Branch, Status, Conclusion string }
	if err := r.db.WithContext(ctx).Table("pipeline_runs").Select("pipeline_runs.external_id as id, pipeline_runs.status, pipeline_runs.conclusion, pull_requests.branch").Joins("LEFT JOIN pull_requests ON pull_requests.repository_id = pipeline_runs.repository_id AND pull_requests.number = pipeline_runs.pr_number").Where("pipeline_runs.repository_id = ?", repo.ID).Order("pipeline_runs.started_at DESC").Limit(100).Find(&pipelines).Error; err != nil {
		return o, err
	}
	for _, v := range pipelines {
		o.Pipelines = append(o.Pipelines, PipelineObservation{ID: v.ID, Branch: v.Branch, Status: v.Status, Conclusion: v.Conclusion})
	}
	var prs []struct {
		Number              int64
		State               string
		MergedAt, UpdatedAt *time.Time
	}
	if err := r.db.WithContext(ctx).Table("pull_requests").Select("number, state, merged_at, updated_at").Where("repository_id = ?", repo.ID).Order("updated_at DESC").Limit(100).Find(&prs).Error; err != nil {
		return o, err
	}
	for _, v := range prs {
		updated := time.Time{}
		if v.UpdatedAt != nil {
			updated = *v.UpdatedAt
		}
		o.PullRequests = append(o.PullRequests, PullRequestObservation{Number: v.Number, State: v.State, Merged: v.MergedAt != nil, UpdatedAt: updated})
	}
	return o, nil
}

func (r *Runtime) enabledChannels(ctx context.Context, org string) (map[string]notification.Notifier, error) {
	rows, err := NewRepository(r.db).ListChannels(ctx, org)
	if err != nil {
		return nil, err
	}
	out := make(map[string]notification.Notifier)
	for _, c := range rows {
		if !c.Enabled {
			continue
		}
		n, ok := notifierFor(c)
		if ok {
			out[c.Kind] = n
		}
	}
	return out, nil
}

func notifierFor(c Channel) (notification.Notifier, bool) {
	get := os.Getenv
	switch c.Kind {
	case "telegram":
		return notification.Telegram{Token: get(c.TelegramTokenEnvRef), ChatID: get(c.TelegramChatIDEnvRef)}, get(c.TelegramTokenEnvRef) != "" && get(c.TelegramChatIDEnvRef) != ""
	case "slack":
		return notification.Slack{WebhookURL: get(c.SlackWebhookURLEnvRef)}, get(c.SlackWebhookURLEnvRef) != ""
	case "smtp":
		return notification.SMTPTransport{Host: get(c.SMTPHostEnvRef), Port: get(c.SMTPPortEnvRef), Username: get(c.SMTPUsernameEnvRef), Password: get(c.SMTPPasswordEnvRef), From: get(c.SMTPFromEnvRef), To: strings.Split(get(c.SMTPToEnvRef), ","), Client: notification.Client{}}, get(c.SMTPHostEnvRef) != "" && get(c.SMTPPortEnvRef) != "" && get(c.SMTPFromEnvRef) != "" && get(c.SMTPToEnvRef) != ""
	default:
		return nil, false
	}
}
