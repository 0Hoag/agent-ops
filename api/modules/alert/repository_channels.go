package alert

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrInvalidChannel = errors.New("invalid alert channel")

var channelKinds = map[string]bool{"telegram": true, "slack": true, "smtp": true}

func (r *Repository) GetChannel(ctx context.Context, organizationID, kind string) (*Channel, error) {
	if err := validateChannel(organizationID, kind); err != nil {
		return nil, err
	}
	var row Channel
	err := r.db.WithContext(ctx).Table("alert_channels").Where("organization_id = ? AND kind = ?", organizationID, kind).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alert channel: %w", err)
	}
	return &row, nil
}

func (r *Repository) ListChannels(ctx context.Context, organizationID string) ([]Channel, error) {
	if err := validateOrganizationID(organizationID); err != nil {
		return nil, err
	}
	var rows []Channel
	if err := r.db.WithContext(ctx).Table("alert_channels").Where("organization_id = ?", organizationID).Order("kind ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list alert channels: %w", err)
	}
	return rows, nil
}

func (r *Repository) CreateChannel(ctx context.Context, channel Channel) error {
	if err := validateChannel(channel.OrganizationID, channel.Kind); err != nil {
		return err
	}
	if channel.ID == "" {
		channel.ID = uuid.NewString()
	}
	if err := r.db.WithContext(ctx).Exec(channelWriteSQL, channel.ID, channel.OrganizationID, channel.Kind, channel.Enabled, channel.TelegramTokenEnvRef, channel.TelegramChatIDEnvRef, channel.SlackWebhookURLEnvRef, channel.SMTPHostEnvRef, channel.SMTPPortEnvRef, channel.SMTPUsernameEnvRef, channel.SMTPPasswordEnvRef, channel.SMTPFromEnvRef, channel.SMTPToEnvRef).Error; err != nil {
		return fmt.Errorf("create alert channel: %w", err)
	}
	return nil
}

func (r *Repository) UpdateChannel(ctx context.Context, channel Channel) error {
	if err := validateChannel(channel.OrganizationID, channel.Kind); err != nil {
		return err
	}
	const query = `UPDATE alert_channels SET enabled = ?, telegram_token_env_ref = ?, telegram_chat_id_env_ref = ?, slack_webhook_url_env_ref = ?, smtp_host_env_ref = ?, smtp_port_env_ref = ?, smtp_username_env_ref = ?, smtp_password_env_ref = ?, smtp_from_env_ref = ?, smtp_to_env_ref = ?, updated_at = NOW() WHERE organization_id = ? AND kind = ?`
	if err := r.db.WithContext(ctx).Exec(query, channel.Enabled, channel.TelegramTokenEnvRef, channel.TelegramChatIDEnvRef, channel.SlackWebhookURLEnvRef, channel.SMTPHostEnvRef, channel.SMTPPortEnvRef, channel.SMTPUsernameEnvRef, channel.SMTPPasswordEnvRef, channel.SMTPFromEnvRef, channel.SMTPToEnvRef, channel.OrganizationID, channel.Kind).Error; err != nil {
		return fmt.Errorf("update alert channel: %w", err)
	}
	return nil
}

func (r *Repository) DeleteChannel(ctx context.Context, organizationID, kind string) error {
	if err := validateChannel(organizationID, kind); err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Exec("DELETE FROM alert_channels WHERE organization_id = ? AND kind = ?", organizationID, kind).Error; err != nil {
		return fmt.Errorf("delete alert channel: %w", err)
	}
	return nil
}

const channelWriteSQL = `INSERT INTO alert_channels (id, organization_id, kind, enabled, telegram_token_env_ref, telegram_chat_id_env_ref, slack_webhook_url_env_ref, smtp_host_env_ref, smtp_port_env_ref, smtp_username_env_ref, smtp_password_env_ref, smtp_from_env_ref, smtp_to_env_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func validateChannel(organizationID, kind string) error {
	if err := validateOrganizationID(organizationID); err != nil {
		return fmt.Errorf("%w: organization id: %v", ErrInvalidChannel, err)
	}
	if !channelKinds[kind] {
		return fmt.Errorf("%w: unsupported kind %q", ErrInvalidChannel, kind)
	}
	return nil
}
