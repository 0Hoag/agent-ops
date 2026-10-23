package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
)

func TestValidEntity(t *testing.T) {
	cases := []struct {
		name  string
		event scm.Event
		want  bool
	}{
		{"issue", scm.Event{Issue: &scm.Issue{Number: 1, Title: "t", State: "open"}}, true},
		{"issue zero", scm.Event{Issue: &scm.Issue{Title: "t", State: "open"}}, false},
		{"issue state empty", scm.Event{Issue: &scm.Issue{Number: 1, Title: "t"}}, false},
		{"pull", scm.Event{PullRequest: &scm.PullRequest{Number: 1, Title: "t", State: "open", HeadSHA: "sha", HeadBranch: "main"}}, true},
		{"pull sha empty", scm.Event{PullRequest: &scm.PullRequest{Number: 1, Title: "t", State: "open", HeadBranch: "main"}}, false},
		{"run", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "sha", Status: "completed", StartedAt: time.Now()}}, true},
		{"run zero", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "0", SHA: "sha", Status: "completed", StartedAt: time.Now()}}, false},
		{"run status empty", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "sha", StartedAt: time.Now()}}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := validEntity(test.event); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

type fakeProvider struct{}

func (fakeProvider) VerifyWebhook(context.Context, http.Header, []byte) error { return nil }
func (fakeProvider) ParseEvent(_ context.Context, kind string, h http.Header, payload []byte) (scm.Event, error) {
	var b struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Issue struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(payload, &b); err != nil {
		return scm.Event{}, err
	}
	if b.Repository.FullName == "" {
		return scm.Event{}, errors.New("missing repository")
	}
	return scm.Event{Repository: b.Repository.FullName, DeliveryID: h.Get("X-GitHub-Delivery"), EventType: kind, Issue: &scm.Issue{Number: b.Issue.Number, Title: b.Issue.Title, State: b.Issue.State}}, nil
}
func (fakeProvider) Backfill(context.Context, string) (scm.BackfillResult, error) {
	return scm.BackfillResult{}, nil
}

type fakeRegistry struct{}

func (fakeRegistry) Build(string, scm.Config) (scm.SCMProvider, error) { return fakeProvider{}, nil }

type badRegistry struct{}

func (badRegistry) Build(string, scm.Config) (scm.SCMProvider, error) {
	return nil, errors.New("internal secret detail")
}

func TestIngestRejectsRepoMismatchAndWrongSecret(t *testing.T) {
	s := NewService(nil, fakeRegistry{})
	body := []byte(`{"repository":{"full_name":"org/a"},"issue":{"number":1,"title":"t","state":"open"}}`)
	h := http.Header{"X-GitHub-Delivery": []string{"d1"}, "X-GitHub-Event": []string{"issues"}}
	_, err := s.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", h, body)
	if err != ErrCandidateMismatch {
		t.Fatalf("mismatch err=%v", err)
	}
	mac := hmac.New(sha256.New, []byte("secret-a"))
	_, _ = mac.Write(body)
	h.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	_, err = s.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", h, body)
	if err != ErrCandidateMismatch {
		t.Fatalf("signature err=%v", err)
	}
}
func TestUnknownProviderReturnsSafeError(t *testing.T) {
	_, err := NewService(nil, badRegistry{}).Ingest(context.Background(), "unknown", "repo", "", "", "", http.Header{}, nil)
	if err != ErrRejected {
		t.Fatalf("err=%v", err)
	}
	if err.Error() == "internal secret detail" {
		t.Fatal("internal details leaked")
	}
}
