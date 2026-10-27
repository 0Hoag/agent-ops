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
	"strings"

	"github.com/0Hoag/agent-ops/api/pkg/transcript"
)

type submitter struct {
	endpoint, token, repository, wave, worker, harness string
	client                                             *http.Client
}

func (s submitter) submit(ctx context.Context, session transcript.Session) error {
	p := map[string]any{"repository_id": s.repository, "wave": s.wave, "worker": s.worker, "model": session.Model, "issue_numbers": []int64{}, "input_tokens": session.Usage.Input, "cached_input_tokens": session.Usage.CachedInput, "cache_write_tokens": session.Usage.CacheWrite, "output_tokens": session.Usage.Output, "reasoning_tokens": session.Usage.Reasoning, "started_at": session.Started, "ended_at": session.Ended, "harness": s.harness, "session_id": session.ID}
	b, err := json.Marshal(p)
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
		return fmt.Errorf("submit session %q: %w", session.ID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("submit session %q: HTTP %s", session.ID, resp.Status)
	}
	return nil
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fail(err.Error())
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: agentops push ... | watch ...")
	}
	if args[0] == "watch" {
		return runWatch(args[1:])
	}
	if args[0] != "push" {
		return fmt.Errorf("usage: agentops push --harness codex|opencode|claude <file>")
	}
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	harness := fs.String("harness", "", "transcript harness: codex, opencode, or claude")
	endpoint := fs.String("endpoint", env("AGENT_RUN_API_URL", "http://127.0.0.1:8080/api/v1/agent-runs"), "agent-run API URL")
	repository := fs.String("repository-id", os.Getenv("AGENT_RUN_REPOSITORY_ID"), "repository UUID")
	wave := fs.String("wave", "", "wave metadata")
	worker := fs.String("worker", "", "worker metadata")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("invalid push flags")
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("exactly one transcript file is required")
	}
	if *harness != "codex" && *harness != "opencode" && *harness != "claude" {
		return fmt.Errorf("--harness must be codex, opencode, or claude")
	}
	token := os.Getenv("AGENT_RUN_TOKEN")
	if token == "" {
		return fmt.Errorf("AGENT_RUN_TOKEN is required")
	}
	if *repository == "" {
		return fmt.Errorf("--repository-id or AGENT_RUN_REPOSITORY_ID is required")
	}
	if *wave == "" {
		return fmt.Errorf("--wave is required")
	}
	if *worker == "" {
		return fmt.Errorf("--worker is required")
	}
	f, err := os.Open(filepath.Clean(fs.Arg(0)))
	if err != nil {
		return fmt.Errorf("open transcript: %w", err)
	}
	defer func() { _ = f.Close() }()
	var sessions []transcript.Session
	switch *harness {
	case "codex":
		sessions, err = transcript.ParseCodex(f)
	case "opencode":
		sessions, err = transcript.ParseOpenCode(f)
	case "claude":
		sessions, err = transcript.ParseClaude(f)
	}
	if err != nil {
		return fmt.Errorf("parse transcript: %w", err)
	}
	if len(sessions) == 0 {
		return fmt.Errorf("transcript contains no usable sessions")
	}
	s := submitter{*endpoint, token, *repository, *wave, *worker, *harness, http.DefaultClient}
	for _, session := range sessions {
		if err := s.submit(context.Background(), session); err != nil {
			return err
		}
	}
	return nil
}
func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
