package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const fixture = `{"type":"assistant","uuid":"a1","sessionId":"session-b","timestamp":"2026-10-24T01:00:00Z","message":{"model":"claude-test","usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":2}}}
{"type":"assistant","uuid":"a1","sessionId":"session-b","timestamp":"2026-10-24T01:00:00Z","message":{"model":"claude-test","usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":2}},"message_content":"DO NOT LOG"}
{"type":"assistant","uuid":"b1","sessionId":"session-a","timestamp":"2026-10-24T02:00:00Z","message":{"model":"claude-test","usage":{"input_tokens":7,"output_tokens":8,"cache_read_input_tokens":5,"cache_creation_input_tokens":6}}}
`

func TestParseTranscriptDeduplicatesStreamingAndSeparatesSessions(t *testing.T) {
	sessions, err := parseTranscript(strings.NewReader(fixture))
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	first := sessions[0]
	require.Equal(t, "session-a", first.ID)
	require.Equal(t, usage{Input: 7, CachedInput: 5, CacheWrite: 6, Output: 8}, first.Usage)
	second := sessions[1]
	require.Equal(t, "session-b", second.ID)
	require.Equal(t, usage{Input: 10, CachedInput: 3, CacheWrite: 2, Output: 4}, second.Usage)
}

func TestSubmitPayloadCheckpointAfterSuccessAndDeduplicatesAcrossInvocations(t *testing.T) {
	var requests int
	var bodies []string
	var headers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/agent-runs", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		headers = append(headers, r.Header.Get("X-Agent-Run-Token"))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	cpPath := filepath.Join(dir, "checkpoint.json")
	require.NoError(t, os.WriteFile(path, []byte(fixture), 0600))
	f, err := os.Open(path)
	require.NoError(t, err)
	sessions, err := parseTranscript(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	s := apiSubmitter{client: server.Client(), endpoint: server.URL + "/api/v1/agent-runs", token: "top-secret", repository: "00000000-0000-0000-0000-000000000001", wave: "W3d", worker: "test"}
	cp, err := loadCheckpoint(cpPath)
	require.NoError(t, err)
	for _, session := range sessions {
		require.NoError(t, s.Submit(context.Background(), session))
		cp.Sessions[session.ID] = true
		require.NoError(t, saveCheckpoint(cpPath, cp))
	}
	cp, err = loadCheckpoint(cpPath)
	require.NoError(t, err)
	for _, session := range sessions {
		if !cp.Sessions[session.ID] {
			require.NoError(t, s.Submit(context.Background(), session))
		}
	}
	require.Equal(t, 2, requests)
	require.Equal(t, []string{"top-secret", "top-secret"}, headers)
	wantPayloads := []map[string]any{
		{"repository_id": "00000000-0000-0000-0000-000000000001", "wave": "W3d", "worker": "test", "model": "claude-test", "issue_numbers": []any{}, "input_tokens": float64(7), "cached_input_tokens": float64(5), "cache_write_tokens": float64(6), "output_tokens": float64(8), "reasoning_tokens": float64(0), "started_at": "2026-10-24T02:00:00Z", "ended_at": "2026-10-24T02:00:00Z"},
		{"repository_id": "00000000-0000-0000-0000-000000000001", "wave": "W3d", "worker": "test", "model": "claude-test", "issue_numbers": []any{}, "input_tokens": float64(10), "cached_input_tokens": float64(3), "cache_write_tokens": float64(2), "output_tokens": float64(4), "reasoning_tokens": float64(0), "started_at": "2026-10-24T01:00:00Z", "ended_at": "2026-10-24T01:00:00Z"},
	}
	for i, body := range bodies {
		require.NotContains(t, body, "DO NOT LOG")
		require.NotContains(t, body, "top-secret")
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		require.Equal(t, wantPayloads[i], got)
	}
}

func TestFailedSubmitLeavesSessionRetryable(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "no", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	s := apiSubmitter{client: server.Client(), endpoint: server.URL, token: "secret", repository: "repo", wave: "W3d", worker: "test"}
	sess := session{ID: "s", Model: "m", Started: time.Now(), Ended: time.Now()}
	cpPath := filepath.Join(t.TempDir(), "checkpoint.json")
	cp, err := loadCheckpoint(cpPath)
	require.NoError(t, err)
	require.Error(t, s.Submit(context.Background(), sess))
	require.False(t, cp.Sessions["s"])
	require.NoError(t, s.Submit(context.Background(), sess))
	cp.Sessions[sess.ID] = true
	require.NoError(t, saveCheckpoint(cpPath, cp))
	reloaded, err := loadCheckpoint(cpPath)
	require.NoError(t, err)
	require.True(t, reloaded.Sessions["s"])
	require.Equal(t, 2, requests)
}
