package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPStdioToolsCallUsesAuthenticatedAPIContracts(t *testing.T) {
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(r.Context()))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/board" {
			_, _ = w.Write([]byte(`{"data":[{"number":7}],"meta":{"total":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"wave-1","state":"active"}}`))
	}))
	defer server.Close()
	in := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}\n" +
		"{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n" +
		"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\",\"params\":{}}\n" +
		"{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"list_issues\",\"arguments\":{\"repository_id\":\"repo-1\",\"page\":2,\"per_page\":25}}}\n" +
		"{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"wave_status\",\"arguments\":{\"wave_id\":\"wave/1\"}}}\n")
	var out bytes.Buffer
	if err := (&mcpServer{apiURL: server.URL, token: "jwt-secret", fiveMinutesPath: "FIVE-MINUTES.md", client: server.Client()}).serve(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, v)
	}
	if len(responses) != 4 || responses[0]["id"].(float64) != 1 || responses[1]["id"].(float64) != 2 {
		t.Fatalf("responses=%s", out.String())
	}
	if len(requests) != 2 {
		t.Fatalf("requests=%d", len(requests))
	}
	if requests[0].URL.RequestURI() != "/api/v1/board?page=2&per_page=25&repository_id=repo-1" {
		t.Fatalf("board URI=%s", requests[0].URL.RequestURI())
	}
	if requests[1].URL.RequestURI() != "/api/v1/waves/wave%2F1" {
		t.Fatalf("wave URI=%s", requests[1].URL.RequestURI())
	}
	for _, r := range requests {
		if r.Header.Get("Authorization") != "Bearer jwt-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
	}
	if strings.Contains(out.String(), "jwt-secret") {
		t.Fatal("token leaked to stdout")
	}
}

func TestMCPAddsCostsAndSimilarFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "FIVE-MINUTES.md")
	if err := os.WriteFile(path, []byte("# Retry failure\nThe provider timeout was recorded.\n# Other\nNo match.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requests []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Clone(r.Context()))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/insights" {
			_, _ = w.Write([]byte(`{"data":{"suggestions":[{"key":"retry","message":"Provider timeout"}],"send_back":{"evidence":[{"event_type":"provider_timeout"}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`[{"wave":"W15d","cost_usd":null}]`))
	}))
	defer server.Close()
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"my_costs","arguments":{"repository_id":"repo-1","wave":"W15d","issue_number":84}}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"similar_failures","arguments":{"query":"TIMEOUT","repository_id":"repo-1"}}}` + "\n")
	var out bytes.Buffer
	if err := (&mcpServer{apiURL: server.URL, token: "jwt", fiveMinutesPath: path, client: server.Client()}).serve(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "my_costs") || !strings.Contains(out.String(), "similar_failures") || !strings.Contains(out.String(), "Retry failure") || !strings.Contains(out.String(), "provider_timeout") {
		t.Fatalf("output=%s", out.String())
	}
	if len(requests) != 2 || requests[0].URL.RequestURI() != "/api/v1/agent-runs/aggregate?issue_number=84&repository_id=repo-1&wave=W15d" || requests[1].URL.RequestURI() != "/api/v1/insights?repository_id=repo-1" {
		t.Fatalf("requests=%v", requests)
	}
	for _, r := range requests {
		if r.Header.Get("Authorization") != "Bearer jwt" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
	}
}

func TestMCPPropagatesUnauthorizedAndValidatesArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_issues","arguments":{"repository_id":"repo"}}}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_issues","arguments":{}}}` + "\n")
	if err := (&mcpServer{apiURL: server.URL, token: "jwt", fiveMinutesPath: "FIVE-MINUTES.md", client: server.Client()}).serve(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "401 Unauthorized") || !strings.Contains(out.String(), "repository_id") {
		t.Fatalf("output=%s", out.String())
	}
}
