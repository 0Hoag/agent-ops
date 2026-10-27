package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCodex(t *testing.T) {
	s, err := ParseCodex(strings.NewReader(readFixture(t, "codex.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 {
		t.Fatalf("sessions=%d", len(s))
	}
	got := s[0]
	want := (Usage{140, 20, 3, 12, 2})
	if got.ID != "t1" || got.Model != "gpt" || got.Usage != want {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestParseOpenCode(t *testing.T) {
	s, err := ParseOpenCode(strings.NewReader(readFixture(t, "opencode.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].Usage != (Usage{18, 5, 5, 5, 3}) {
		t.Fatalf("got %#v", s)
	}
	wantStart := time.UnixMilli(1761526800000)
	wantEnd := time.UnixMilli(1761526860000)
	if s[0].Started.IsZero() || !s[0].Started.Equal(wantStart) || !s[0].Ended.Equal(wantEnd) {
		t.Fatalf("times=%s..%s want %s..%s", s[0].Started, s[0].Ended, wantStart, wantEnd)
	}
}

func TestParseClaudeDeduplicatesMessagesAndRetainsNoContent(t *testing.T) {
	s, err := ParseClaude(strings.NewReader(readFixture(t, "claude.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].Usage != (Usage{15, 5, 3, 3, 0}) || s[0].Model != "claude" {
		t.Fatalf("got %#v", s)
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseMalformedAndReadFailure(t *testing.T) {
	if _, err := ParseCodex(strings.NewReader("{")); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error=%v", err)
	}
	if _, err := ParseCodex(failingReader{}); err == nil || !strings.Contains(err.Error(), "read transcript") {
		t.Fatalf("error=%v", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, testReadError{} }

type testReadError struct{}

func (testReadError) Error() string { return "synthetic read failure" }
