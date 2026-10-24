package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type usage struct{ Input, CachedInput, CacheWrite, Output, Reasoning int64 }
type session struct {
	ID, Model      string
	Started, Ended time.Time
	Usage          usage
	seen           map[string]bool
}
type transcriptLine struct {
	Type, UUID, SessionID, Model, Timestamp string
	ParentToolUseID                         string `json:"parent_tool_use_id"`
	Message                                 struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			CacheRead    int64 `json:"cache_read_input_tokens"`
			CacheWrite   int64 `json:"cache_creation_input_tokens"`
			Reasoning    int64 `json:"reasoning_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func parseTranscript(r io.Reader) ([]session, error) {
	sessions := map[string]*session{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var item transcriptLine
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", line, err)
		}
		id := strings.TrimSpace(item.SessionID)
		if id == "" {
			continue
		}
		if item.UUID == "" {
			continue
		}
		s := sessions[id]
		if s == nil {
			s = &session{ID: id, seen: map[string]bool{}}
			sessions[id] = s
		}
		if s.seen[item.UUID] {
			continue
		}
		s.seen[item.UUID] = true
		stamp, err := time.Parse(time.RFC3339Nano, item.Timestamp)
		if err == nil {
			if s.Started.IsZero() || stamp.Before(s.Started) {
				s.Started = stamp
			}
			if stamp.After(s.Ended) {
				s.Ended = stamp
			}
		}
		if item.Model != "" {
			s.Model = item.Model
		}
		if item.Message.Model != "" {
			s.Model = item.Message.Model
		}
		u := item.Message.Usage
		s.Usage.Input += u.InputTokens
		s.Usage.Output += u.OutputTokens
		s.Usage.CachedInput += u.CacheRead
		s.Usage.CacheWrite += u.CacheWrite
		s.Usage.Reasoning += u.Reasoning
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	out := make([]session, 0, len(sessions))
	for _, s := range sessions {
		if s.Model == "" {
			s.Model = "unknown"
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

type checkpoint struct {
	Sessions map[string]bool `json:"sessions"`
}

func loadCheckpoint(path string) (checkpoint, error) {
	c := checkpoint{Sessions: map[string]bool{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read checkpoint: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("invalid checkpoint: %w", err)
	}
	if c.Sessions == nil {
		c.Sessions = map[string]bool{}
	}
	return c, nil
}
func saveCheckpoint(path string, c checkpoint) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit checkpoint: %w", err)
	}
	return nil
}
