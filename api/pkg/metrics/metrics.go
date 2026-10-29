package metrics

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Clock func() time.Time

type Metrics struct {
	mu       sync.Mutex
	now      Clock
	requests map[requestKey]requestValue
	webhooks map[string]float64
	queues   map[string]int
}
type requestKey struct{ method, route, status string }
type requestValue struct {
	count   uint64
	seconds float64
}

func New(now Clock) *Metrics {
	if now == nil {
		now = time.Now
	}
	return &Metrics{now: now, requests: map[requestKey]requestValue{}, webhooks: map[string]float64{}, queues: map[string]int{}}
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := m.now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}
		m.observeRequest(r.Method, route, rw.status, m.now().Sub(started))
	})
}
func (m *Metrics) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := m.now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		m.observeRequest(c.Request.Method, route, c.Writer.Status(), m.now().Sub(started))
	}
}
func (m *Metrics) observeRequest(method, route string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := requestKey{method, route, strconv.Itoa(status)}
	v := m.requests[k]
	v.count++
	v.seconds += duration.Seconds()
	m.requests[k] = v
}
func (m *Metrics) ObserveWebhook(provider string, started time.Time) {
	lag := m.now().Sub(started).Seconds()
	if lag < 0 {
		lag = 0
	}
	m.mu.Lock()
	m.webhooks[provider] += lag
	m.mu.Unlock()
}
func (m *Metrics) SetQueueDepth(repository string, depth int) {
	m.mu.Lock()
	m.queues[repository] = depth
	m.mu.Unlock()
}
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(m.Exposition()))
	})
}
func (m *Metrics) Exposition() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	b.WriteString("# TYPE agent_ops_http_request_duration_seconds summary\n")
	keys := make([]requestKey, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
	for _, k := range keys {
		v := m.requests[k]
		labels := fmt.Sprintf("method=\"%s\",route=\"%s\",status=\"%s\"", k.method, k.route, k.status)
		fmt.Fprintf(&b, "agent_ops_http_request_duration_seconds_count{%s} %d\nagent_ops_http_request_duration_seconds_sum{%s} %g\n", labels, v.count, labels, v.seconds)
	}
	b.WriteString("# TYPE agent_ops_webhook_lag_seconds summary\n")
	providers := make([]string, 0, len(m.webhooks))
	for p := range m.webhooks {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	for _, p := range providers {
		fmt.Fprintf(&b, "agent_ops_webhook_lag_seconds_sum{provider=\"%s\"} %g\n", p, m.webhooks[p])
	}
	b.WriteString("# TYPE agent_ops_websocket_queue_depth gauge\n")
	depth := 0
	for _, value := range m.queues {
		depth += value
	}
	fmt.Fprintf(&b, "agent_ops_websocket_queue_depth %d\n", depth)
	return b.String()
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(p)
}
