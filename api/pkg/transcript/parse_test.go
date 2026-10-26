package transcript

import (
	"strings"
	"testing"
)

func TestParseCodex(t *testing.T) {
	r := `{"type":"thread.started","thread_id":"t1"}
{"type":"turn.completed","thread_id":"t1","model":"gpt","timestamp":"2026-10-27T01:00:00Z","usage":{"input_tokens":100,"cached_input_tokens":20,"cache_write_input_tokens":3,"output_tokens":7,"reasoning_output_tokens":2}}
{"type":"noise","thread_id":"t1"}
{"type":"turn.completed","thread_id":"t1","timestamp":"2026-10-27T01:01:00Z","usage":{"input_tokens":40,"output_tokens":5}}
`
	s, err := ParseCodex(strings.NewReader(r))
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
	r := `{"type":"text","sessionID":"s1","part":{"text":"ignored"}}
{"type":"step_finish","sessionID":"s1","part":{"tokens":{"input":11,"output":2,"reasoning":1,"cache":{"read":4,"write":5}}}}
{"type":"tool","sessionID":"s1"}
{"type":"step_finish","sessionID":"s1","part":{"tokens":{"input":7,"output":3,"reasoning":2,"cache":{"read":1,"write":0}}}}
`
	s, err := ParseOpenCode(strings.NewReader(r))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].Usage != (Usage{18, 5, 5, 5, 3}) {
		t.Fatalf("got %#v", s)
	}
}

func TestParseClaudeDeduplicatesMessagesAndRetainsNoContent(t *testing.T) {
	r := `{"type":"assistant","uuid":"m1","sessionId":"c1","timestamp":"2026-10-27T01:00:00Z","message":{"model":"claude","content":[{"text":"secret"}],"usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":4,"cache_creation_input_tokens":3}}}
{"type":"assistant","uuid":"m1","sessionId":"c1","message":{"content":[{"text":"secret"}],"usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":4,"cache_creation_input_tokens":3}}}
{"type":"user","uuid":"u1","sessionId":"c1"}
{"type":"assistant","uuid":"m2","sessionId":"c1","message":{"usage":{"input_tokens":5,"output_tokens":1,"cache_read_input_tokens":1}}}
`
	s, err := ParseClaude(strings.NewReader(r))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].Usage != (Usage{15, 5, 3, 3, 0}) || s[0].Model != "claude" {
		t.Fatalf("got %#v", s)
	}
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
