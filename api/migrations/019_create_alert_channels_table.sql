CREATE TABLE alert_channels (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('telegram', 'slack', 'smtp')),
    enabled BOOLEAN NOT NULL DEFAULT false,
    telegram_token_env_ref TEXT NOT NULL DEFAULT '',
    telegram_chat_id_env_ref TEXT NOT NULL DEFAULT '',
    slack_webhook_url_env_ref TEXT NOT NULL DEFAULT '',
    smtp_host_env_ref TEXT NOT NULL DEFAULT '',
    smtp_port_env_ref TEXT NOT NULL DEFAULT '',
    smtp_username_env_ref TEXT NOT NULL DEFAULT '',
    smtp_password_env_ref TEXT NOT NULL DEFAULT '',
    smtp_from_env_ref TEXT NOT NULL DEFAULT '',
    smtp_to_env_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, kind)
);

CREATE INDEX alert_channels_organization_idx ON alert_channels (organization_id);
