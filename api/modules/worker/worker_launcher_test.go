package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLifecycleMatchesRepositoryLauncherContract(t *testing.T) {
	root, err := os.MkdirTemp(".", "worker-launcher-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(root)) })

	worktree := filepath.Join(root, "permitted-worktree")
	require.NoError(t, os.Mkdir(worktree, 0755))
	record := filepath.Join(root, "invocation")
	script := filepath.Join(root, "launcher.sh")
	contents := "#!/bin/sh\nprintf '%s\\n%s\\n%s\\n%s\\n' \"$#\" \"$1\" \"$2\" \"$3\" > \"" + record + "\"\nprintf '%s\\n' \"$PWD\" >> \"" + record + "\"\n"
	require.NoError(t, os.WriteFile(script, []byte(contents), 0755))
	brief := filepath.Join(root, "brief.md")
	require.NoError(t, os.WriteFile(brief, []byte("brief"), 0644))

	relScript, err := filepath.Rel(".", script)
	require.NoError(t, err)
	relBrief, err := filepath.Rel(".", brief)
	require.NoError(t, err)
	e, err := New(Config{
		QueueSize: 1, Concurrency: 1, ScriptRoots: []string{"."},
		WorktreeRoots: []string{worktree}, Scripts: map[string]string{"fixture": relScript},
	})
	require.NoError(t, err)
	defer e.Close()

	j, err := e.Enqueue(Request{OrganizationID: "org-a", Wave: "W14h", Branch: "feat/w14h-launcher-adapter", Brief: relBrief, Worktree: worktree, Script: "fixture"})
	require.NoError(t, err)
	waitForJob(t, e, j.ID, "succeeded")

	data, err := os.ReadFile(record)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Equal(t, []string{"3", "W14h", "feat/w14h-launcher-adapter", filepath.Join(e.wd, relBrief), e.wd}, lines)
}

func waitForJob(t *testing.T, e *Executor, id, state string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := e.Get(id)
		if err == nil && job.State == state {
			return
		}
		time.Sleep(time.Millisecond * 5)
	}
	job, _ := e.Get(id)
	t.Fatalf("worker did not reach %s: %#v", state, job)
}
