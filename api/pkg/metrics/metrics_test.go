package metrics

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	if !strings.Contains(body, `route="/items/:id"`) {
		t.Fatalf("expected route template, got %s", body)
	}
	if strings.Contains(body, "/items/1") {
		t.Fatal("raw URL leaked")
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
