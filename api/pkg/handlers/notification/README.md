# Notification adapters

The package provides Telegram Bot API, Slack incoming webhook, and SMTP adapters. Credentials are read only from environment variables by `LoadConfig(os.Getenv)`: `NOTIFICATION_TELEGRAM_TOKEN`, `NOTIFICATION_TELEGRAM_CHAT_ID`, `NOTIFICATION_SLACK_WEBHOOK_URL`, and `NOTIFICATION_SMTP_HOST`, `NOTIFICATION_SMTP_PORT`, `NOTIFICATION_SMTP_USERNAME`, `NOTIFICATION_SMTP_PASSWORD`, `NOTIFICATION_SMTP_FROM`, `NOTIFICATION_SMTP_TO`.

Telegram and Slack use context-aware HTTP POST requests. Tests use local `httptest` servers and never contact providers. SMTP uses the normal authenticated SMTP envelope and supports an injected transport for deterministic tests; production deployments should use an authenticated TLS SMTP transport. No adapter logs credentials or provider response bodies beyond a bounded error snippet.
