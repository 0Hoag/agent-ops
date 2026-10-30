package worker

import (
	"strings"
	"sync"
)

const logHistoryLimit = 256
const logSubscriberBuffer = 32

type logStream struct {
	mu      sync.Mutex
	history []string
	subs    map[chan string]struct{}
	closed  bool
}

func newLogStream() *logStream { return &logStream{subs: make(map[chan string]struct{})} }

func (s *logStream) Write(p []byte) (int, error) {
	lines := strings.Split(strings.ReplaceAll(string(p), "\r\n", "\n"), "\n")
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range lines {
		if line == "" || s.closed {
			continue
		}
		if len(s.history) == logHistoryLimit {
			s.history = s.history[1:]
		}
		s.history = append(s.history, line)
		for ch := range s.subs {
			select {
			case ch <- line:
			default:
			}
		}
	}
	return len(p), nil
}

func (s *logStream) subscribe() ([]string, <-chan string, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := append([]string(nil), s.history...)
	ch := make(chan string, logSubscriberBuffer)
	if s.closed {
		close(ch)
		return history, ch, func() {}
	}
	s.subs[ch] = struct{}{}
	var once sync.Once
	return history, ch, func() { once.Do(func() { s.unsubscribe(ch) }) }
}
func (s *logStream) unsubscribe(ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[ch]; ok {
		delete(s.subs, ch)
		close(ch)
	}
}
func (s *logStream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for ch := range s.subs {
		close(ch)
	}
	s.subs = make(map[chan string]struct{})
}
