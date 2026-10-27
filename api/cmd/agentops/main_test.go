package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/transcript"
)

func TestRunRejectsInvalidHarnessInputAndConfiguration(t *testing.T) {
	old := os.Getenv("AGENT_RUN_TOKEN")
	t.Cleanup(func() { _ = os.Setenv("AGENT_RUN_TOKEN", old) })
	_ = os.Setenv("AGENT_RUN_TOKEN", "token")
	fixture := filepath.Join("..", "..", "pkg", "transcript", "testdata", "codex.jsonl")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"invalid harness", []string{"push", "--harness", "other", fixture}, "--harness must be"},
		{"missing input", []string{"push", "--harness", "codex"}, "exactly one transcript"},
		{"missing token", []string{"push", "--harness", "codex", "--repository-id", "7e57d004-2b97-0e7a-b45f-5387367791cd", "--wave", "w", "--worker", "x", fixture}, "AGENT_RUN_TOKEN"},
		{"missing repository", []string{"push", "--harness", "codex", "--wave", "w", "--worker", "x", fixture}, "repository-id"},
		{"missing wave", []string{"push", "--harness", "codex", "--repository-id", "7e57d004-2b97-0e7a-b45f-5387367791cd", "--worker", "x", fixture}, "--wave"},
		{"missing worker", []string{"push", "--harness", "codex", "--repository-id", "7e57d004-2b97-0e7a-b45f-5387367791cd", "--wave", "w", fixture}, "--worker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "missing token" {
				_ = os.Unsetenv("AGENT_RUN_TOKEN")
			}
			if tt.name != "missing token" {
				_ = os.Setenv("AGENT_RUN_TOKEN", "token")
			}
			err := run(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestSubmitRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusBadRequest) }))
	defer server.Close()
	err := (submitter{endpoint: server.URL, token: "token", client: server.Client()}).submit(t.Context(), sessionFixture())
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("error=%v", err)
	}
}

func sessionFixture() transcript.Session {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	return transcript.Session{ID: "session", Model: "model", Started: now, Ended: now, Usage: transcript.Usage{Input: 1}}
}
