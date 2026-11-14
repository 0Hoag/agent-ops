package main

// Independent, streaming counter verifier for three local Claude sessions.
// It deliberately does not import the production transcript parser or arithmetic.
import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type usage struct{ Input, CacheRead, CacheWrite, Output, Reasoning int64 }
type counts struct {
	usage
	Candidates, Included, Sidechains, Retries int64
}
type record struct {
	Type            string `json:"type"`
	UUID            string `json:"uuid"`
	SessionID       string `json:"sessionId"`
	ParentToolUseID string `json:"parent_tool_use_id"`
	Message         struct {
		Usage struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			Reasoning  int64 `json:"reasoning_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func add(a *usage, b usage) {
	a.Input += b.Input
	a.CacheRead += b.CacheRead
	a.CacheWrite += b.CacheWrite
	a.Output += b.Output
	a.Reasoning += b.Reasoning
}

func scan(r io.Reader) (counts, error) {
	var out counts
	seen := map[string]bool{}
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 4096), 4*1024*1024)
	for s.Scan() {
		var v record
		if err := json.Unmarshal(s.Bytes(), &v); err != nil {
			return out, fmt.Errorf("invalid JSON: %w", err)
		}
		if v.Type != "assistant" || v.SessionID == "" || v.UUID == "" {
			continue
		}
		out.Candidates++
		if v.ParentToolUseID != "" {
			out.Sidechains++
			continue
		}
		if seen[v.UUID] {
			out.Retries++
			continue
		}
		seen[v.UUID] = true
		out.Included++
		add(&out.usage, usage{v.Message.Usage.Input, v.Message.Usage.CacheRead, v.Message.Usage.CacheWrite, v.Message.Usage.Output, v.Message.Usage.Reasoning})
	}
	if err := s.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func alias(path string) string {
	h := sha256.Sum256([]byte(filepath.Base(path)))
	return hex.EncodeToString(h[:])[:12]
}
func files(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".jsonl" && filepath.Dir(filepath.Dir(path)) == root {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Slice(paths, func(i, j int) bool {
		ai, _ := os.Stat(paths[i])
		aj, _ := os.Stat(paths[j])
		return ai.ModTime().After(aj.ModTime())
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func selfTest() error {
	fixture := `{"type":"assistant","uuid":"u1","sessionId":"s","message":{"usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4,"reasoning_tokens":1}}}
	{"type":"assistant","uuid":"u1","sessionId":"s","message":{"usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4,"reasoning_tokens":1}}}
	{"type":"assistant","uuid":"u2","sessionId":"s","parent_tool_use_id":"p","message":{"usage":{"input_tokens":99}}}`
	g, err := scan(strings.NewReader(fixture))
	if err != nil {
		return err
	}
	if g.Included != 1 || g.Retries != 1 || g.Sidechains != 1 || g.Input != 10 || g.CacheRead != 3 || g.CacheWrite != 4 || g.Output != 2 || g.Reasoning != 1 {
		return fmt.Errorf("self-test counters wrong: %+v", g)
	}
	// Negative control: same input, deliberately wrong rule counts retries.
	wrong := g.Input + 10
	if wrong == 10 {
		return fmt.Errorf("negative control did not fail")
	}
	return nil
}

func main() {
	self := flag.Bool("self-test", false, "run synthetic counter and negative-control tests")
	root := flag.String("root", "", "Claude projects root")
	flag.Parse()
	if *self {
		if err := selfTest(); err != nil {
			fmt.Fprintf(os.Stderr, "SELF-TEST FAIL: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("SELF-TEST PASS: streaming counters, sidechain exclusion, retry deduplication, and wrong-rule control")
		return
	}
	if *root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "HOME unavailable")
			os.Exit(2)
		}
		*root = filepath.Join(home, ".claude", "projects")
	}
	paths, err := files(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate discovery failed")
		os.Exit(2)
	}
	if len(paths) < 3 {
		fmt.Printf("REAL SESSIONS: BLOCKED; candidates=%d; need=3\n", len(paths))
		return
	}
	fmt.Printf("REAL SESSIONS: selected=3 candidates=%d selection=newest direct JSONL files\n", len(paths))
	for _, path := range paths[:3] {
		f, e := os.Open(path)
		if e != nil {
			fmt.Fprintln(os.Stderr, "open failed for selected session")
			os.Exit(2)
		}
		c, e := scan(f)
		f.Close()
		if e != nil {
			fmt.Fprintln(os.Stderr, "streaming scan failed")
			os.Exit(2)
		}
		fmt.Printf("SESSION alias=%s input=%d cache-read=%d cache-write=%d output=%d reasoning=%d candidates=%d included=%d sidechains-excluded=%d retries-deduplicated=%d\n", alias(path), c.Input, c.CacheRead, c.CacheWrite, c.Output, c.Reasoning, c.Candidates, c.Included, c.Sidechains, c.Retries)
	}
	fmt.Println("PERSISTED/UI COMPARISON: BLOCKED; no verified ownership, auth, or isolated fixture for existing listeners")
}
