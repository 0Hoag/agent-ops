package notification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestTelegramFixture(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottok/sendMessage" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	err := (Telegram{"tok", "chat", srv.URL, srv.Client()}).Notify(context.Background(), Message{Body: "hello"})
	if err != nil || !strings.Contains(got, "hello") {
		t.Fatalf("telegram: %v %s", err, got)
	}
}

func TestSlackFixture(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hook" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := (Slack{BaseURL: srv.URL + "/hook", Client: srv.Client()}).Notify(context.Background(), Message{Subject: "subject", Body: "hello"}); err != nil || !strings.Contains(got, "hello") {
		t.Fatalf("slack: %v %s", err, got)
	}
}
func TestSlackFailureFixture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hook" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("fixture failure"))
	}))
	defer srv.Close()
	if err := (Slack{BaseURL: srv.URL + "/hook", Client: srv.Client()}).Notify(context.Background(), Message{Body: "x"}); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatal(err)
	}
}

type fakeSMTP struct {
	address, from string
	to            []string
	msg           []byte
	err           error
}

func (f *fakeSMTP) SendMail(host, port, username, password, from string, to []string, msg []byte) error {
	f.address, f.from, f.to, f.msg = host+":"+port, from, to, msg
	return f.err
}
func TestSMTPFixture(t *testing.T) {
	f := &fakeSMTP{}
	s := SMTPTransport{"smtp.test", "2525", "u", "p", "from@test", []string{"to@test"}, f}
	if err := s.Notify(context.Background(), Message{"subject", "body"}); err != nil || f.address != "smtp.test:2525" || !strings.Contains(string(f.msg), "Subject: subject\r\n\r\nbody") {
		t.Fatalf("smtp: %v %#v", err, f)
	}
}
func TestSMTPFailure(t *testing.T) {
	f := &fakeSMTP{err: errors.New("protocol rejected")}
	s := SMTPTransport{Host: "smtp.test", Port: "25", From: "a", To: []string{"b"}, Client: f}
	if err := s.Notify(context.Background(), Message{}); err == nil || !strings.Contains(err.Error(), "protocol rejected") {
		t.Fatal(err)
	}
}
func TestLoadConfigUsesEnvironment(t *testing.T) {
	t.Setenv("NOTIFICATION_SLACK_WEBHOOK_URL", "local")
	c, err := LoadConfig(os.Getenv)
	if err != nil || c.SlackWebhookURL != "local" {
		t.Fatalf("config: %#v %v", c, err)
	}
}
