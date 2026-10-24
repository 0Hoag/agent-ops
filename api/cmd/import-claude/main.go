package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type apiSubmitter struct {
	client                                    *http.Client
	endpoint, token, repository, wave, worker string
}

func (s apiSubmitter) Submit(ctx context.Context, in session) error {
	payload := struct {
		RepositoryID      string    `json:"repository_id"`
		Wave              string    `json:"wave"`
		Worker            string    `json:"worker"`
		Model             string    `json:"model"`
		IssueNumbers      []int64   `json:"issue_numbers"`
		InputTokens       int64     `json:"input_tokens"`
		CachedInputTokens int64     `json:"cached_input_tokens"`
		CacheWriteTokens  int64     `json:"cache_write_tokens"`
		OutputTokens      int64     `json:"output_tokens"`
		ReasoningTokens   int64     `json:"reasoning_tokens"`
		StartedAt         time.Time `json:"started_at"`
		EndedAt           time.Time `json:"ended_at"`
	}{s.repository, s.wave, s.worker, in.Model, nil, in.Usage.Input, in.Usage.CachedInput, in.Usage.CacheWrite, in.Usage.Output, in.Usage.Reasoning, in.Started, in.Ended}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Run-Token", s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("submit session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("submit session: HTTP %s", resp.Status)
	}
	return nil
}

func main() {
	path := flag.String("path", "", "Claude JSONL transcript path")
	checkpointPath := flag.String("checkpoint", "", "durable successful-session checkpoint path")
	endpoint := flag.String("endpoint", "http://127.0.0.1:8080/api/v1/agent-runs", "agent-runs endpoint")
	repository := flag.String("repository-id", "", "repository UUID")
	wave := flag.String("wave", "W3d", "wave name")
	worker := flag.String("worker", "claude-import", "worker name")
	flag.Parse()
	if *path == "" || *checkpointPath == "" || *repository == "" {
		fail("-path, -checkpoint, and -repository-id are required")
	}
	token := os.Getenv("AGENT_RUN_TOKEN")
	if token == "" {
		fail("AGENT_RUN_TOKEN is required")
	}
	f, err := os.Open(filepath.Clean(*path))
	if err != nil {
		fail(err.Error())
	}
	defer func() { _ = f.Close() }()
	sessions, err := parseTranscript(f)
	if err != nil {
		fail(err.Error())
	}
	cp, err := loadCheckpoint(*checkpointPath)
	if err != nil {
		fail(err.Error())
	}
	s := apiSubmitter{http.DefaultClient, *endpoint, token, *repository, *wave, *worker}
	for _, sess := range sessions {
		if cp.Sessions[sess.ID] {
			continue
		}
		if err := s.Submit(context.Background(), sess); err != nil {
			fail(err.Error())
		}
		cp.Sessions[sess.ID] = true
		if err := saveCheckpoint(*checkpointPath, cp); err != nil {
			fail(err.Error())
		}
	}
}
func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
