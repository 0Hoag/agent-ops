package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/gin-gonic/gin"
)

func TestValidEntity(t *testing.T) {
	cases := []struct {
		name  string
		event scm.Event
		want  bool
	}{
		{"issue", scm.Event{Issue: &scm.Issue{Number: 1, Title: "t", State: "open"}}, true},
		{"issue id zero", scm.Event{Issue: &scm.Issue{Title: "t", State: "open"}}, false},
		{"issue state empty", scm.Event{Issue: &scm.Issue{Number: 1, Title: "t"}}, false},
		{"pull", scm.Event{PullRequest: &scm.PullRequest{Number: 1, Title: "t", State: "open", HeadSHA: "sha", HeadBranch: "main"}}, true},
		{"pull sha empty", scm.Event{PullRequest: &scm.PullRequest{Number: 1, Title: "t", State: "open", HeadBranch: "main"}}, false},
		{"run", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "sha", Status: "completed"}}, true},
		{"run id zero", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "0", SHA: "sha", Status: "completed"}}, false},
		{"run status empty", scm.Event{PipelineRun: &scm.PipelineRun{ExternalID: "42", SHA: "sha"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := validEntity(c.event); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

type fakeProvider struct{}

func (fakeProvider) VerifyWebhook(context.Context, http.Header, []byte) error { return nil }
func (fakeProvider) ParseEvent(_ context.Context, kind string, h http.Header, payload []byte) (scm.Event, error) {
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
	if body.Repository.FullName == "" {
		return scm.Event{}, errors.New("missing repository")
	}
	return scm.Event{Repository: body.Repository.FullName, DeliveryID: h.Get("X-GitHub-Delivery"), EventType: kind, Issue: &scm.Issue{Number: body.Issue.Number, Title: body.Issue.Title, State: body.Issue.State}}, nil
}
func (fakeProvider) Backfill(context.Context, string) (scm.BackfillResult, error) {
	return scm.BackfillResult{}, nil
}

type fakeRegistry struct{}

func (fakeRegistry) Build(string, scm.Config) (scm.SCMProvider, error) { return fakeProvider{}, nil }

func TestControllerBoundsCandidates(t *testing.T) {
	c := NewController(nil, map[string][]Config{"github": make([]Config, 33)})
	r := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(r)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	ctx.Params = gin.Params{{Key: "provider", Value: "github"}}
	c.Handle(ctx)
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", r.Code)
	}
}

func TestIngestRejectsMismatchedRepoAndWrongSecretBeforeWrites(t *testing.T) {
	s := NewService(nil, fakeRegistry{})
	body := []byte(`{"repository":{"full_name":"org/a"},"issue":{"number":1,"title":"t","state":"open"}}`)
	h := http.Header{"X-GitHub-Event": []string{"issues"}, "X-GitHub-Delivery": []string{"d1"}}
	_, err := s.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", h, body)
	if err != ErrRejected {
		t.Fatalf("repo mismatch err=%v", err)
	}
	mac := hmac.New(sha256.New, []byte("secret-a"))
	_, _ = mac.Write(body)
	h.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	_, err = s.Ingest(context.Background(), "github", "org/b", "secret-b", "", "", h, body)
	if err != ErrRejected {
		t.Fatalf("wrong secret err=%v", err)
	}
}

type badRegistry struct{}

func (badRegistry) Build(string, scm.Config) (scm.SCMProvider, error) {
	return nil, errors.New("secret database internals")
}
func TestUnknownProviderErrorIsSafe(t *testing.T) {
	_, err := NewService(nil, badRegistry{}).Ingest(context.Background(), "unknown", "repo", "", "", "", http.Header{}, nil)
	if err != ErrRejected {
		t.Fatalf("error=%v", err)
	}
	if err.Error() == "secret database internals" {
		t.Fatal("internal detail exposed")
	}
}
