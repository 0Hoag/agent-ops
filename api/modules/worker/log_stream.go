package worker

import "sync"

const logHistoryLimit = 256
const logSubscriberBuffer = 32

type logStream struct {
	mu      sync.Mutex
	history []string
	subs    map[chan string]struct{}
	pending string
	closed  bool
}

type logWriter struct {
	stream  *logStream
	pending string
}

func newLogWriter(stream *logStream) *logWriter { return &logWriter{stream: stream} }

func (w *logWriter) Write(p []byte) (int, error) {
	w.pending += string(p)
	for {
		idx := -1
		for i, b := range w.pending {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := w.pending[:idx]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		w.pending = w.pending[idx+1:]
		w.stream.mu.Lock()
		w.stream.publish(line)
		w.stream.mu.Unlock()
	}
	return len(p), nil
}

func (w *logWriter) close() {
	if w.pending == "" {
		return
	}
	w.stream.mu.Lock()
	w.stream.publish(w.pending)
	w.stream.mu.Unlock()
	w.pending = ""
}

func newLogStream() *logStream { return &logStream{subs: make(map[chan string]struct{})} }

func (s *logStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return len(p), nil
	}
	s.pending += string(p)
	s.flushCompleteLines()
	return len(p), nil
}

func (s *logStream) flushCompleteLines() {
	for {
		idx := -1
		for i, b := range s.pending {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		line := s.pending[:idx]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		s.pending = s.pending[idx+1:]
		s.publish(line)
	}
}

func (s *logStream) publish(line string) {
	if line == "" || s.closed {
		return
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
	s.flushCompleteLines()
	if s.pending != "" {
		s.publish(s.pending)
		s.pending = ""
	}
	s.closed = true
	for ch := range s.subs {
		close(ch)
	}
	s.subs = make(map[chan string]struct{})
}
