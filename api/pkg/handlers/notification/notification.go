package notification

import (
	"context"
	"fmt"
)

type Message struct {
	Subject string
	Body    string
}

type Notifier interface {
	Notify(context.Context, Message) error
}

type Config struct {
	TelegramToken, TelegramChatID                                    string
	SlackWebhookURL                                                  string
	SMTPHost, SMTPPort, SMTPUsername, SMTPPassword, SMTPFrom, SMTPTo string
}

func LoadConfig(getenv func(string) string) (Config, error) {
	c := Config{TelegramToken: getenv("NOTIFICATION_TELEGRAM_TOKEN"), TelegramChatID: getenv("NOTIFICATION_TELEGRAM_CHAT_ID"), SlackWebhookURL: getenv("NOTIFICATION_SLACK_WEBHOOK_URL"), SMTPHost: getenv("NOTIFICATION_SMTP_HOST"), SMTPPort: getenv("NOTIFICATION_SMTP_PORT"), SMTPUsername: getenv("NOTIFICATION_SMTP_USERNAME"), SMTPPassword: getenv("NOTIFICATION_SMTP_PASSWORD"), SMTPFrom: getenv("NOTIFICATION_SMTP_FROM"), SMTPTo: getenv("NOTIFICATION_SMTP_TO")}
	if c.TelegramToken == "" && c.SlackWebhookURL == "" && c.SMTPHost == "" {
		return Config{}, fmt.Errorf("at least one notification channel is required")
	}
	if c.TelegramToken != "" && c.TelegramChatID == "" {
		return Config{}, fmt.Errorf("NOTIFICATION_TELEGRAM_CHAT_ID is required")
	}
	if c.SMTPHost != "" && (c.SMTPPort == "" || c.SMTPFrom == "" || c.SMTPTo == "") {
		return Config{}, fmt.Errorf("SMTP port, from, and to are required")
	}
	return c, nil
}
