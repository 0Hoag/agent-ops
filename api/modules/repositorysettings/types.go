package repositorysettings

type CreateRequest struct {
	Provider       string            `json:"provider"`
	FullName       string            `json:"full_name"`
	WebhookSecret  string            `json:"webhook_secret"`
	BackfillToken  string            `json:"backfill_token"`
	GitLabBaseURL  string            `json:"gitlab_base_url"`
	StatusMappings map[string]string `json:"status_mappings"`
}
type UpdateRequest struct {
	WebhookSecret  *string           `json:"webhook_secret"`
	BackfillToken  *string           `json:"backfill_token"`
	GitLabBaseURL  *string           `json:"gitlab_base_url"`
	StatusMappings map[string]string `json:"status_mappings"`
}
type View struct {
	ID             string            `json:"id"`
	Provider       string            `json:"provider"`
	FullName       string            `json:"full_name"`
	GitLabBaseURL  string            `json:"gitlab_base_url,omitempty"`
	StatusMappings map[string]string `json:"status_mappings"`
	WebhookURL     string            `json:"webhook_url"`
	WebhookSecret  string            `json:"webhook_secret,omitempty"`
	BackfillToken  string            `json:"backfill_token,omitempty"`
}
