package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogStreamFramesSplitChunksAndCRLF(t *testing.T) {
	s := newLogStream()
	history, lines, unsubscribe, err := subscribeForTest(s)
	require.NoError(t, err)
	require.Empty(t, history)
	defer unsubscribe()

	w := newLogWriter(s)
	_, _ = w.Write([]byte("hel"))
	_, _ = w.Write([]byte("lo\r"))
	_, _ = w.Write([]byte("\nsecond\n"))

	require.Equal(t, "hello", <-lines)
	require.Equal(t, "second", <-lines)
}

func TestLogStreamKeepsStdoutAndStderrFragmentsIndependent(t *testing.T) {
	s := newLogStream()
	out, errOut := newLogWriter(s), newLogWriter(s)
	_, _ = out.Write([]byte("out"))
	_, _ = errOut.Write([]byte("err"))
	_, _ = out.Write([]byte("put\n"))
	_, _ = errOut.Write([]byte("or\n"))

	require.Equal(t, []string{"output", "error"}, s.history)
}

func TestLogStreamFlushesFinalPartialLineOnceOnWriterAndStreamClose(t *testing.T) {
	s := newLogStream()
	w := newLogWriter(s)
	_, _ = w.Write([]byte("final"))
	w.close()
	w.close()
	s.close()
	require.Equal(t, []string{"final"}, s.history)
}

func subscribeForTest(s *logStream) ([]string, <-chan string, func(), error) {
	history, lines, unsubscribe := s.subscribe()
	return history, lines, unsubscribe, nil
}
