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
}

type CreatedView struct {
	View
	WebhookSecret string `json:"webhook_secret"`
	BackfillToken string `json:"backfill_token"`
}

type ListResponse struct {
	Status int    `json:"status"`
	Data   []View `json:"data"`
}
type ViewResponse struct {
	Status int  `json:"status"`
	Data   View `json:"data"`
}
type CreatedResponse struct {
	Status int         `json:"status"`
	Data   CreatedView `json:"data"`
}
