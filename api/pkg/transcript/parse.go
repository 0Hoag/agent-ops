package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type codexRecord struct {
	Type      string `json:"type"`
	ThreadID  string `json:"thread_id"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	Timestamp string `json:"timestamp"`
	Usage     struct {
		Input      int64 `json:"input_tokens"`
		Cached     int64 `json:"cached_input_tokens"`
		CacheWrite int64 `json:"cache_write_input_tokens"`
		Output     int64 `json:"output_tokens"`
		Reasoning  int64 `json:"reasoning_output_tokens"`
	} `json:"usage"`
}

// ParseCodex parses turn.completed records. Codex input_tokens already
// includes cached input; CachedInput is retained as a source counter and must
// not be added to Input again by consumers.
func ParseCodex(r io.Reader) ([]Session, error) {
	activeThreadID := ""
	return parse(r, func(line []byte, sessions map[string]*Session, seen map[string]map[string]bool) error {
		var v codexRecord
		if err := json.Unmarshal(line, &v); err != nil {
			return err
		}
		if v.Type == "thread.started" {
			activeThreadID = strings.TrimSpace(v.ThreadID)
			return nil
		}
		if v.Type != "turn.completed" {
			return nil
		}
		id := strings.TrimSpace(v.SessionID)
		if id == "" {
			id = strings.TrimSpace(v.ThreadID)
		}
		if id == "" {
			id = activeThreadID
		}
		if id == "" {
			return fmt.Errorf("turn.completed record has no session identity")
		}
		key := string(line)
		s := getSession(sessions, id)
		if seen[id] == nil {
			seen[id] = map[string]bool{}
		}
		if seen[id][key] {
			return nil
		}
		seen[id][key] = true
		if v.Model != "" {
			s.Model = v.Model
		}
		addTime(s, v.Timestamp)
		u := Usage{Input: v.Usage.Input, CachedInput: v.Usage.Cached, CacheWrite: v.Usage.CacheWrite, Output: v.Usage.Output, Reasoning: v.Usage.Reasoning}
		s.Usage.Input += u.Input
		s.Usage.CachedInput += v.Usage.Cached
		s.Usage.CacheWrite += v.Usage.CacheWrite
		s.Usage.Output += v.Usage.Output
		s.Usage.Reasoning += v.Usage.Reasoning
		t := parseTime(v.Timestamp)
		s.Steps = append(s.Steps, Step{Started: t, Ended: t, Usage: u, CacheHit: u.CachedInput > 0})
		return nil
	})
}

type openCodeRecord struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	SessionID string `json:"sessionID"`
	Part      struct {
		Tokens struct {
			Input     int64 `json:"input"`
			Output    int64 `json:"output"`
			Reasoning int64 `json:"reasoning"`
			Cache     struct {
				Read  int64 `json:"read"`
				Write int64 `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
	} `json:"part"`
}

// ParseOpenCode parses step_finish records. OpenCode input and cache counters
// are separate source counters, unlike Codex's input/cache relationship.
func ParseOpenCode(r io.Reader) ([]Session, error) {
	return parse(r, func(line []byte, sessions map[string]*Session, seen map[string]map[string]bool) error {
		var v openCodeRecord
		if err := json.Unmarshal(line, &v); err != nil {
			return err
		}
		if v.Type != "step_finish" && v.Type != "step-finish" {
			return nil
		}
		id := strings.TrimSpace(v.SessionID)
		if id == "" {
			return fmt.Errorf("step_finish record has no session identity")
		}
		s := getSession(sessions, id)
		if seen[id] == nil {
			seen[id] = map[string]bool{}
		}
		key := string(line)
		if seen[id][key] {
			return nil
		}
		seen[id][key] = true
		t := v.Part.Tokens
		s.Usage.Input += t.Input
		s.Usage.Output += t.Output
		s.Usage.Reasoning += t.Reasoning
		s.Usage.CachedInput += t.Cache.Read
		s.Usage.CacheWrite += t.Cache.Write
		ts := addUnixMillisValue(v.Timestamp)
		s.Steps = append(s.Steps, Step{Started: ts, Ended: ts, Usage: Usage{Input: t.Input, CachedInput: t.Cache.Read, CacheWrite: t.Cache.Write, Output: t.Output, Reasoning: t.Reasoning}, CacheHit: t.Cache.Read > 0})
		if v.Timestamp != 0 {
			addUnixMillis(s, v.Timestamp)
		}
		return nil
	})
}

type claudeRecord struct {
	Type, UUID, SessionID, Model, Timestamp string
	Message                                 struct {
		Model string `json:"model"`
		Usage struct {
			Input     int64 `json:"input_tokens"`
			Output    int64 `json:"output_tokens"`
			Read      int64 `json:"cache_read_input_tokens"`
			Write     int64 `json:"cache_creation_input_tokens"`
			Reasoning int64 `json:"reasoning_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ParseClaude parses assistant message usage and deduplicates repeated JSONL
// representations by UUID. Prompt and tool content are never decoded/stored.
func ParseClaude(r io.Reader) ([]Session, error) {
	return parse(r, func(line []byte, sessions map[string]*Session, seen map[string]map[string]bool) error {
		var v claudeRecord
		if err := json.Unmarshal(line, &v); err != nil {
			return err
		}
		if v.Type != "assistant" || strings.TrimSpace(v.SessionID) == "" || v.UUID == "" {
			return nil
		}
		id := strings.TrimSpace(v.SessionID)
		s := getSession(sessions, id)
		if seen[id] == nil {
			seen[id] = map[string]bool{}
		}
		if seen[id][v.UUID] {
			return nil
		}
		seen[id][v.UUID] = true
		if v.Model != "" {
			s.Model = v.Model
		}
		if v.Message.Model != "" {
			s.Model = v.Message.Model
		}
		addTime(s, v.Timestamp)
		u := v.Message.Usage
		s.Usage = Usage{s.Usage.Input + u.Input, s.Usage.CachedInput + u.Read, s.Usage.CacheWrite + u.Write, s.Usage.Output + u.Output, s.Usage.Reasoning + u.Reasoning}
		t := parseTime(v.Timestamp)
		s.Steps = append(s.Steps, Step{Started: t, Ended: t, Usage: Usage{Input: u.Input, CachedInput: u.Read, CacheWrite: u.Write, Output: u.Output, Reasoning: u.Reasoning}, CacheHit: u.Read > 0})
		return nil
	})
}

func parse(r io.Reader, fn func([]byte, map[string]*Session, map[string]map[string]bool) error) ([]Session, error) {
	if r == nil {
		return nil, fmt.Errorf("transcript reader is nil")
	}
	sessions := map[string]*Session{}
	seen := map[string]map[string]bool{}
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 4096), 4*1024*1024)
	line := 0
	for scan.Scan() {
		line++
		if err := fn(scan.Bytes(), sessions, seen); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	out := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func getSession(m map[string]*Session, id string) *Session {
	if m[id] == nil {
		m[id] = &Session{ID: id}
	}
	return m[id]
}
func addTime(s *Session, raw string) {
	if raw == "" {
		return
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return
	}
	if s.Started.IsZero() || t.Before(s.Started) {
		s.Started = t
	}
	if t.After(s.Ended) {
		s.Ended = t
	}
}

func addUnixMillis(s *Session, millis int64) {
	t := addUnixMillisValue(millis)
	if s.Started.IsZero() || t.Before(s.Started) {
		s.Started = t
	}
	if t.After(s.Ended) {
		s.Ended = t
	}
}
func parseTime(raw string) time.Time { t, _ := time.Parse(time.RFC3339Nano, raw); return t }
func addUnixMillisValue(millis int64) time.Time {
	if millis == 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis)
}
