package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Telegram struct {
	Token, ChatID, BaseURL string
	Client                 *http.Client
}

func (t Telegram) Notify(ctx context.Context, m Message) error {
	b, _ := json.Marshal(map[string]string{"chat_id": t.ChatID, "text": m.Body})
	return postJSON(ctx, t.Client, strings.TrimRight(t.BaseURL, "/")+"/bot"+t.Token+"/sendMessage", b)
}

type Slack struct {
	WebhookURL, BaseURL string
	Client              *http.Client
}

func (s Slack) Notify(ctx context.Context, m Message) error {
	b, _ := json.Marshal(map[string]string{"text": m.Subject + "\n" + m.Body})
	u := s.WebhookURL
	if s.BaseURL != "" {
		u = s.BaseURL
	}
	return postJSON(ctx, s.Client, u, b)
}

func postJSON(ctx context.Context, client *http.Client, url string, body []byte) error {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer res.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(res.Body, 4096))
	if readErr != nil {
		return fmt.Errorf("read notification response: %w", readErr)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("notification provider returned %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	return nil
}
