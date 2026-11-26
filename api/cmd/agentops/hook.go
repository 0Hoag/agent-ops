package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/0Hoag/agent-ops/api/pkg/transcript"
)

type claudeHookEvent struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	Event          string `json:"hook_event_name"`
}

// runHook consumes one Claude lifecycle event. It intentionally parses only
// the path and session named by the event; no transcript content is retained.
func runHook(args []string, input io.Reader, client *http.Client) error {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	endpoint := fs.String("endpoint", env("AGENT_RUN_API_URL", "http://127.0.0.1:8080/api/v1/agent-runs"), "agent-run API URL")
	repository := fs.String("repository-id", os.Getenv("AGENT_RUN_REPOSITORY_ID"), "repository UUID")
	wave := fs.String("wave", "", "wave metadata")
	worker := fs.String("worker", "", "worker metadata")
	model := fs.String("model", os.Getenv("AGENT_RUN_MODEL"), "model metadata fallback")
	billingType := fs.String("billing-type", "api", "billing type: api or subscription")
	issues := fs.String("issue-numbers", os.Getenv("AGENT_RUN_ISSUES"), "JSON issue-number array")
	branch := fs.String("branch", "", "source branch override")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid hook flags")
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("hook reads exactly one event from stdin")
	}
	if *repository != "" && (*wave == "" || *worker == "") {
		return fmt.Errorf("--wave and --worker are required with --repository-id")
	}
	if *repository == "" {
		*wave, *worker = "local", "claude"
	}
	if *billingType != "api" && *billingType != "subscription" {
		return fmt.Errorf("--billing-type must be api or subscription")
	}
	if os.Getenv("AGENT_RUN_TOKEN") == "" {
		return fmt.Errorf("AGENT_RUN_TOKEN is required")
	}
	var event claudeHookEvent
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode Claude hook event: %w", err)
	}
	if strings.TrimSpace(event.SessionID) == "" || strings.TrimSpace(event.TranscriptPath) == "" || strings.TrimSpace(event.CWD) == "" {
		return fmt.Errorf("hook event requires non-empty session_id, transcript_path, and cwd")
	}
	if event.Event != "" && event.Event != "Stop" {
		return fmt.Errorf("unsupported Claude hook event %q; expected Stop", event.Event)
	}
	info, err := os.Stat(event.CWD)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("hook cwd %q is not an accessible directory", event.CWD)
	}
	path := filepath.Clean(event.TranscriptPath)
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open hook transcript %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	sessions, err := transcript.ParseClaude(file)
	if err != nil {
		return fmt.Errorf("parse hook transcript %q: %w", path, err)
	}
	var selected *transcript.Session
	for i := range sessions {
		if sessions[i].ID == event.SessionID {
			selected = &sessions[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("hook session_id %q was not found in transcript %q", event.SessionID, path)
	}
	if strings.TrimSpace(selected.Model) == "" {
		selected.Model = strings.TrimSpace(*model)
	}
	if selected.Model == "" {
		return fmt.Errorf("hook session %q has no model metadata; provide --model or AGENT_RUN_MODEL", selected.ID)
	}
	var issueNumbers []int64
	if strings.TrimSpace(*issues) != "" {
		if err := json.Unmarshal([]byte(*issues), &issueNumbers); err != nil {
			return fmt.Errorf("invalid issue-numbers JSON: %w", err)
		}
	}
	s := submitter{endpoint: *endpoint, token: os.Getenv("AGENT_RUN_TOKEN"), repository: *repository, wave: *wave, worker: *worker, harness: "claude", branch: *branch, cwd: event.CWD, billingType: *billingType, issues: issueNumbers, client: client}
	return s.submit(context.Background(), *selected)
}
