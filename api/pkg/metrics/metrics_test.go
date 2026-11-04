package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestScrapeHasBoundedRequestLabelsAndValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Unix(10, 0)
	m := New(func() time.Time { return now })
	r := gin.New()
	r.Use(m.GinMiddleware())
	r.GET("/items/:id", func(c *gin.Context) { c.Status(201) })
	for _, id := range []string{"1", "2"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/items/"+id, nil))
	}
	body := m.Exposition()
	if !strings.Contains(body, `route="/items/:id"`) || strings.Contains(body, "/items/1") {
		t.Fatalf("matched route regression: %s", body)
	}
}

func TestScrapeBoundsUnmatchedGinRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := New(time.Now)
	r := gin.New()
	r.Use(m.GinMiddleware())
	for _, path := range []string{"/missing/one", `/missing/quote\\path"`} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	body := m.Exposition()
	if strings.Contains(body, "/missing/") {
		t.Fatalf("raw unmatched path leaked: %s", body)
	}
	if got := strings.Count(body, `agent_ops_http_request_duration_seconds_count{method="GET",route="unmatched",status="404"} 2`); got != 1 {
		t.Fatalf("expected bounded count 2, got %d in %s", got, body)
	}
}

func TestScrapeBoundsUnmatchedNetHTTPRoutesAndEscapesLabels(t *testing.T) {
	m := New(time.Now)
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) }))
	for _, path := range []string{"/missing/one", `/missing/quote\\path"`} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	m.ObserveWebhook(`provider\\"`, time.Now())
	body := m.Exposition()
	if strings.Contains(body, "/missing/") {
		t.Fatalf("raw unmatched path leaked: %s", body)
	}
	if got := strings.Count(body, `agent_ops_http_request_duration_seconds_count{method="GET",route="unmatched",status="201"} 2`); got != 1 {
		t.Fatalf("expected bounded count 2, got %d in %s", got, body)
	}
	if !strings.Contains(body, `provider="provider`) || !strings.Contains(body, `\\\\`) || !strings.Contains(body, `\"`) {
		t.Fatalf("expected escaped provider label, got %s", body)
	}
}

func TestWebhookAndQueueValues(t *testing.T) {
	now := time.Unix(10, 0)
	m := New(func() time.Time { return now })
	m.ObserveWebhook("github", now.Add(-2*time.Second))
	m.SetQueueDepth("repo", 3)
	s := m.Exposition()
	for _, want := range []string{"provider=\"github\"", "agent_ops_webhook_lag_seconds_sum{provider=\"github\"} 2", "agent_ops_websocket_queue_depth 3"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %s", want, s)
		}
	}
}
