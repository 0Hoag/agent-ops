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

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
)

func TestValidEntity(t *testing.T) {
	tests := []struct {
		name  string
		event scm.Event
		want  bool
	}{
		{name: "issue", event: scm.Event{Issue: &scm.Issue{Number: 1, Title: "t", State: "open"}}, want: true},
		{name: "zero issue", event: scm.Event{Issue: &scm.Issue{Title: "t", State: "open"}}},
		{name: "missing issue state", event: scm.Event{Issue: &scm.Issue{Number: 1, Title: "t"}}},
		{name: "pull", event: scm.Event{PullRequest: &scm.PullRequest{Number: 2, Title: "t", State: "merged", HeadSHA: "abc", HeadBranch: "main"}}, want: true},
		{name: "missing pull sha", event: scm.Event{PullRequest: &scm.PullRequest{Number: 2, Title: "t", State: "open", HeadBranch: "main"}}},
		{name: "run", event: scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "abc", Status: "completed"}}, want: true},
		{name: "zero run id", event: scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "0", SHA: "abc", Status: "completed"}}},
		{name: "missing run status", event: scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "abc"}}},
		{name: "multiple", event: scm.Event{Issue: &scm.Issue{Number: 1, Title: "t", State: "open"}, PullRequest: &scm.PullRequest{Number: 2, Title: "t", State: "open", HeadSHA: "abc", HeadBranch: "main"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validEntity(test.event); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

type fakeProvider struct{}

func (fakeProvider) VerifyWebhook(context.Context, http.Header, []byte) error { return nil }
func (fakeProvider) ParseEvent(_ context.Context, eventType string, headers http.Header, payload []byte) (scm.Event, error) {
	var body struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Issue struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return scm.Event{}, err
	}
	issue := scm.Issue{Number: body.Issue.Number, Title: body.Issue.Title, State: body.Issue.State}
	if issue.Number == 0 {
		return scm.Event{}, errors.New("invalid issue")
	}
	return scm.Event{Repository: body.Repository.FullName, DeliveryID: headers.Get("X-GitHub-Delivery"), EventType: eventType, Issue: &issue}, nil
}
func (fakeProvider) Backfill(context.Context, string) (scm.BackfillResult, error) {
	return scm.BackfillResult{}, nil
}

type fakeRegistry struct{ err error }

func (registry fakeRegistry) Build(string, scm.Config) (scm.SCMProvider, error) {
	if registry.err != nil {
		return nil, registry.err
	}
	return fakeProvider{}, nil
}

func TestIngestRejectsSpoofedRepositoryBeforeAnyDatabaseWrite(t *testing.T) {
	service := NewService(nil, fakeRegistry{})
	payload := []byte(`{"repository":{"full_name":"org/a"},"issue":{"number":1,"title":"issue","state":"open"}}`)
	headers := http.Header{"X-GitHub-Delivery": []string{"d1"}, "X-GitHub-Event": []string{"issues"}}
	_, err := service.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", headers, payload)
	if err != ErrRejected {
		t.Fatalf("got %v, want ErrRejected", err)
	}
}

func TestIngestRejectsSignatureFromDifferentRepositorySecret(t *testing.T) {
	service := NewService(nil, fakeRegistry{})
	payload := []byte(`{"repository":{"full_name":"org/a"},"issue":{"number":1,"title":"issue","state":"open"}}`)
	digest := hmac.New(sha256.New, []byte("secret-a"))
	_, _ = digest.Write(payload)
	headers := http.Header{"X-GitHub-Delivery": []string{"d1"}, "X-GitHub-Event": []string{"issues"}, "X-Hub-Signature-256": []string{"sha256=" + hex.EncodeToString(digest.Sum(nil))}}
	_, err := service.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", headers, payload)
	if err != ErrRejected {
		t.Fatalf("got %v, want ErrRejected", err)
	}
}

func TestIngestUnknownProviderReturnsSafeError(t *testing.T) {
	service := NewService(nil, fakeRegistry{err: errors.New("secret/database detail")})
	_, err := service.Ingest(context.Background(), "unknown", "org/a", "", "", "", http.Header{}, nil)
	if err != ErrRejected {
		t.Fatalf("got %v, want ErrRejected", err)
	}
	if err.Error() == "secret/database detail" {
		t.Fatal("internal error leaked")
	}
}
