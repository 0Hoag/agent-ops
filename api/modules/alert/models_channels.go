package alert

import "time"

// Channel stores references to environment variables, never resolved credentials.
type Channel struct {
	ID                    string
	OrganizationID        string
	Kind                  string
	Enabled               bool
	TelegramTokenEnvRef   string
	TelegramChatIDEnvRef  string
	SlackWebhookURLEnvRef string
	SMTPHostEnvRef        string
	SMTPPortEnvRef        string
	SMTPUsernameEnvRef    string
	SMTPPasswordEnvRef    string
	SMTPFromEnvRef        string
	SMTPToEnvRef          string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
