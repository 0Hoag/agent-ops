package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/transcript"
)

func TestPollClaudeHandlesAppendPartialRestartAndNoDuplicate(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "org", "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "session.jsonl")
	record := func(uuid string, input int) string {
		b, _ := json.Marshal(map[string]any{"type": "assistant", "uuid": uuid, "sessionId": "s1", "timestamp": "2026-10-27T00:00:00Z", "message": map[string]any{"model": "claude", "usage": map[string]int{"input_tokens": input, "output_tokens": 2}}})
		return string(b) + "\n"
	}
	if err := os.WriteFile(path, []byte(record("a", 3)), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []transcript.Session
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			SessionID string `json:"session_id"`
			Input     int64  `json:"input_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		got = append(got, transcript.Session{ID: p.SessionID, Usage: transcript.Usage{Input: p.Input}})
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	s := submitter{endpoint: server.URL, token: "token", repository: "repo", wave: "W9", worker: "worker", harness: "claude", client: server.Client()}
	cp := watchCheckpoint{Files: map[string]watchFileState{}}
	changed, err := pollClaude(context.Background(), root, &cp, s)
	if err != nil || !changed {
		t.Fatalf("first poll changed=%v err=%v", changed, err)
	}
	if len(got) != 1 || got[0].Usage.Input != 3 {
		t.Fatalf("first payloads=%+v", got)
	}
	changed, err = pollClaude(context.Background(), root, &cp, s)
	if err != nil || changed || len(got) != 1 {
		t.Fatalf("repeat changed=%v err=%v payloads=%+v", changed, err, got)
	}
	partial := record("b", 4)
	if err := os.WriteFile(path, append([]byte(record("a", 3)), []byte(partial[:len(partial)-3])...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := pollClaude(context.Background(), root, &cp, s); err == nil {
		t.Fatal("partial record unexpectedly parsed")
	}
	if err := os.WriteFile(path, []byte(record("a", 3)+partial), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err = pollClaude(context.Background(), root, &cp, s)
	if err != nil || !changed {
		t.Fatalf("completed poll changed=%v err=%v", changed, err)
	}
	if len(got) != 2 || got[1].Usage.Input != 7 {
		t.Fatalf("cumulative payloads=%+v", got)
	}
	checkpoint := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := saveWatchCheckpoint(checkpoint, cp); err != nil {
		t.Fatal(err)
	}
	restarted, err := loadWatchCheckpoint(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	changed, err = pollClaude(context.Background(), root, &restarted, s)
	if err != nil || changed || len(got) != 2 {
		t.Fatalf("restart changed=%v err=%v payloads=%+v", changed, err, got)
	}
}

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

func TestRunSuppliesModelMetadataWithoutChangingSourceModel(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions.jsonl")
	records := `{"type":"step_finish","timestamp":1761526800000,"sessionID":"native","part":{"tokens":{"input":18,"output":5,"reasoning":3,"cache":{"read":5,"write":2}}}}
{"type":"step_finish","timestamp":1761526860000,"sessionID":"native","part":{"tokens":{"input":4,"output":1,"reasoning":0,"cache":{"read":1,"write":0}}}}
`
	if err := os.WriteFile(path, []byte(records), 0o600); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	oldToken := os.Getenv("AGENT_RUN_TOKEN")
	t.Cleanup(func() { _ = os.Setenv("AGENT_RUN_TOKEN", oldToken) })
	_ = os.Setenv("AGENT_RUN_TOKEN", "token")
	err := run([]string{"push", "--harness", "opencode", "--model", "openai/gpt-6-luna", "--endpoint", server.URL, "--repository-id", "repo", "--wave", "W9l", "--worker", "worker", path})
	if err != nil {
		t.Fatal(err)
	}
	if payload["model"] != "openai/gpt-6-luna" || payload["session_id"] != "native" || payload["input_tokens"] != float64(22) || payload["cached_input_tokens"] != float64(6) || payload["output_tokens"] != float64(6) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["started_at"] != "2025-10-27T08:00:00+07:00" || payload["ended_at"] != "2025-10-27T08:01:00+07:00" {
		t.Fatalf("timestamps=%v", payload)
	}

	claudePath := filepath.Join(root, "source.jsonl")
	if err := os.WriteFile(claudePath, []byte(`{"type":"assistant","uuid":"u1","sessionId":"source","timestamp":"2026-10-27T00:00:00Z","message":{"model":"claude-3-7-sonnet","usage":{"input_tokens":1}}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	err = run([]string{"push", "--harness", "claude", "--model", "wrong-fallback", "--endpoint", server.URL, "--repository-id", "repo", "--wave", "W9l", "--worker", "worker", claudePath})
	if err != nil || payload["model"] != "claude-3-7-sonnet" {
		t.Fatalf("source model err=%v payload=%v", err, payload)
	}
}

func TestRunRejectsMissingModelBeforeHTTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"step_finish","timestamp":1761526800000,"sessionID":"native","part":{"tokens":{"input":1}}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; w.WriteHeader(http.StatusCreated) }))
	defer server.Close()
	oldToken, oldModel := os.Getenv("AGENT_RUN_TOKEN"), os.Getenv("AGENT_RUN_MODEL")
	t.Cleanup(func() { _ = os.Setenv("AGENT_RUN_TOKEN", oldToken); _ = os.Setenv("AGENT_RUN_MODEL", oldModel) })
	_ = os.Setenv("AGENT_RUN_TOKEN", "token")
	_ = os.Unsetenv("AGENT_RUN_MODEL")
	err := run([]string{"push", "--harness", "opencode", "--endpoint", server.URL, "--repository-id", "repo", "--wave", "W9l", "--worker", "worker", path})
	if err == nil || !strings.Contains(err.Error(), "--model or AGENT_RUN_MODEL") || requests != 0 {
		t.Fatalf("err=%v requests=%d", err, requests)
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
