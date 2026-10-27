package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/transcript"
)

type watchFileState struct {
	Size     int64             `json:"size"`
	Mod      int64             `json:"mod_time_unix_nano"`
	Sessions map[string]string `json:"session_digests"`
}
type watchCheckpoint struct {
	Files map[string]watchFileState `json:"files"`
}

func runWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", "", "Claude projects directory")
	checkpoint := fs.String("checkpoint", "", "checkpoint path")
	endpoint := fs.String("endpoint", env("AGENT_RUN_API_URL", "http://127.0.0.1:8080/api/v1/agent-runs"), "agent-run API URL")
	repository := fs.String("repository-id", os.Getenv("AGENT_RUN_REPOSITORY_ID"), "repository UUID")
	wave := fs.String("wave", "", "wave metadata")
	worker := fs.String("worker", "", "worker metadata")
	interval := fs.Duration("interval", time.Second, "poll interval")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid watch flags")
	}
	if strings.TrimSpace(*root) == "" || strings.TrimSpace(*checkpoint) == "" || *repository == "" || *wave == "" || *worker == "" {
		return fmt.Errorf("--root, --checkpoint, --repository-id, --wave, and --worker are required")
	}
	if *interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	token := os.Getenv("AGENT_RUN_TOKEN")
	if token == "" {
		return fmt.Errorf("AGENT_RUN_TOKEN is required")
	}
	cp, err := loadWatchCheckpoint(*checkpoint)
	if err != nil {
		return err
	}
	s := submitter{endpoint: *endpoint, token: token, repository: *repository, wave: *wave, worker: *worker, harness: "claude", client: httpClient()}
	ctx, stop := signalContext()
	defer stop()
	for {
		changed, err := pollClaude(ctx, filepath.Clean(*root), &cp, s)
		if err != nil {
			return err
		}
		if changed {
			if err := saveWatchCheckpoint(*checkpoint, cp); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(*interval):
		}
	}
}

func pollClaude(ctx context.Context, root string, cp *watchCheckpoint, s submitter) (bool, error) {
	files, err := claudeFiles(root)
	if err != nil {
		return false, err
	}
	changed := false
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return false, err
		}
		old, exists := cp.Files[path]
		if exists && old.Size == info.Size() && old.Mod == info.ModTime().UnixNano() {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		sessions, parseErr := parseClaudeWatchSnapshot(contents)
		if parseErr != nil {
			return false, fmt.Errorf("parse %s: %w", path, parseErr)
		}
		state := watchFileState{Size: info.Size(), Mod: info.ModTime().UnixNano(), Sessions: map[string]string{}}
		for _, session := range sessions {
			digest, err := sessionDigest(session)
			if err != nil {
				return false, err
			}
			state.Sessions[session.ID] = digest
			if old.Sessions[session.ID] == digest {
				continue
			}
			if err := s.submit(ctx, session); err != nil {
				return false, err
			}
		}
		cp.Files[path] = state
		changed = true
	}
	return changed, nil
}

// parseClaudeWatchSnapshot keeps a live watcher alive while Claude is still
// appending the final JSONL record. ParseClaude remains strict for completed
// input; only an unterminated final line whose decoder reports unexpected EOF
// is deferred until a later poll.
func parseClaudeWatchSnapshot(contents []byte) ([]transcript.Session, error) {
	if len(contents) == 0 || contents[len(contents)-1] == '\n' {
		return transcript.ParseClaude(bytes.NewReader(contents))
	}
	lastNewline := bytes.LastIndexByte(contents, '\n')
	complete := contents[:lastNewline+1]
	trailing := contents[lastNewline+1:]
	var probe json.RawMessage
	if err := json.Unmarshal(trailing, &probe); err == nil {
		return transcript.ParseClaude(bytes.NewReader(contents))
	} else if !strings.Contains(err.Error(), "unexpected end of JSON input") {
		return nil, err
	}
	return transcript.ParseClaude(bytes.NewReader(complete))
}

func claudeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func sessionDigest(s transcript.Session) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func loadWatchCheckpoint(path string) (watchCheckpoint, error) {
	cp := watchCheckpoint{Files: map[string]watchFileState{}}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return cp, nil
	}
	if err != nil {
		return cp, err
	}
	defer func() { _ = f.Close() }()
	if err := json.NewDecoder(f).Decode(&cp); err != nil && !errors.Is(err, io.EOF) {
		return cp, fmt.Errorf("decode checkpoint: %w", err)
	}
	if cp.Files == nil {
		cp.Files = map[string]watchFileState{}
	}
	return cp, nil
}
func saveWatchCheckpoint(path string, cp watchCheckpoint) error {
	b, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".agentops-checkpoint-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var httpClient = func() *http.Client { return http.DefaultClient }

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
