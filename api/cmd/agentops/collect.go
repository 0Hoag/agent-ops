package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/0Hoag/agent-ops/api/pkg/transcript"
)

// collectCheckpoint stores only metadata digests. It deliberately contains no transcript content.
type collectCheckpoint struct {
	Sessions map[string]string `json:"sessions"`
}

func runCollect(args []string) error {
	fs := flag.NewFlagSet("collect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("claude-root", filepath.Join(os.Getenv("HOME"), ".claude", "projects"), "Claude projects directory")
	defaultDB := filepath.Join(os.Getenv("HOME"), ".local", "share", "opencode", "opencode.db")
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		defaultDB = filepath.Join(xdg, "opencode", "opencode.db")
	}
	db := fs.String("opencode-db", env("OPENCODE_DB", defaultDB), "OpenCode SQLite database")
	checkpoint := fs.String("checkpoint", filepath.Join(os.TempDir(), "agentops-collect.json"), "metadata checkpoint")
	_ = fs.Bool("once", false, "scan once and exit (the default)")
	endpoint := fs.String("endpoint", env("AGENT_RUN_API_URL", "http://127.0.0.1:8080/api/v1/agent-runs"), "agent-run API URL")
	wave := fs.String("wave", "local", "wave metadata")
	worker := fs.String("worker", "local", "worker metadata")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if os.Getenv("AGENT_RUN_TOKEN") == "" {
		return fmt.Errorf("AGENT_RUN_TOKEN is required")
	}
	cp, err := loadCollectCheckpoint(*checkpoint)
	if err != nil {
		return err
	}
	s := submitter{endpoint: *endpoint, token: os.Getenv("AGENT_RUN_TOKEN"), wave: *wave, worker: *worker, billingType: "api", client: httpClient()}
	count, skipped := 0, 0
	files, err := claudeFiles(*root)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, path := range files {
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		sessions, e := transcript.ParseClaude(strings.NewReader(string(b)))
		if e != nil {
			return fmt.Errorf("parse %s: %w", path, e)
		}
		for _, session := range sessions {
			if session.Model == "unknown" {
				fmt.Fprintf(os.Stderr, "skip Claude session %s: model metadata missing\n", session.ID)
				skipped++
				continue
			}
			digest, _ := sessionDigest(session)
			key := "claude/" + session.ID
			if cp.Sessions[key] == digest {
				continue
			}
			cs := s
			cs.harness = "claude"
			if err := cs.submit(context.Background(), session); err != nil {
				return err
			}
			cp.Sessions[key] = digest
			count++
		}
	}
	if strings.TrimSpace(*db) != "" {
		sessions, e := readOpenCodeSQLite(*db)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		for _, session := range sessions {
			if session.Model == "unknown" {
				fmt.Fprintf(os.Stderr, "skip OpenCode session %s: model metadata missing\n", session.ID)
				skipped++
				continue
			}
			digest, _ := sessionDigest(session)
			key := "opencode/" + session.ID
			if cp.Sessions[key] == digest {
				continue
			}
			cs := s
			cs.harness = "opencode"
			if err := cs.submit(context.Background(), session); err != nil {
				return err
			}
			cp.Sessions[key] = digest
			count++
		}
	}
	if err := saveCollectCheckpoint(*checkpoint, cp); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "collected %d sessions, skipped %d\n", count, skipped)
	return nil
}

// readOpenCodeSQLite invokes sqlite3 in immutable read-only mode. The CLI is used
// instead of a CGO driver so the API binary remains CGO-free; no write-capable
// connection is ever opened and the query selects metadata/token JSON only.
func readOpenCodeSQLite(path string) ([]transcript.Session, error) {
	query := `SELECT m.session_id AS session_id,p.time_created AS timestamp,p.data AS data,json_extract(m.data,'$.model') AS model FROM part p JOIN message m ON m.id=p.message_id WHERE json_extract(p.data,'$.type') IN ('step-finish','step_finish') ORDER BY p.time_created`
	out, err := exec.Command("sqlite3", "-readonly", "-json", "file:"+filepath.Clean(path)+"?immutable=1", query).Output()
	if err != nil {
		return nil, fmt.Errorf("read OpenCode database read-only: %w", err)
	}
	var rows []struct {
		SessionID string `json:"session_id"`
		Timestamp int64  `json:"timestamp"`
		Data      string `json:"data"`
		Model     string `json:"model"`
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, err
	}
	// Reuse the metadata-only parser after converting SQLite rows to synthetic JSONL.
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		var part struct {
			Tokens json.RawMessage `json:"tokens"`
			Tool   string          `json:"tool"`
		}
		if err := json.Unmarshal([]byte(row.Data), &part); err != nil {
			return nil, err
		}
		lines = append(lines, fmt.Sprintf(`{"type":"step_finish","timestamp":%d,"sessionID":%q,"model":%q,"part":{"tool":%q,"tokens":%s}}`, row.Timestamp, row.SessionID, row.Model, part.Tool, part.Tokens))
	}
	return transcript.ParseOpenCode(strings.NewReader(strings.Join(lines, "\n")))
}

func loadCollectCheckpoint(path string) (collectCheckpoint, error) {
	c := collectCheckpoint{Sessions: map[string]string{}}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	if c.Sessions == nil {
		c.Sessions = map[string]string{}
	}
	return c, nil
}
func saveCollectCheckpoint(path string, c collectCheckpoint) error {
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	tmp := path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
