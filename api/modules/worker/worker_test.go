package worker

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycleUsesAllowlistedExecutableAndCancels(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0755))
	e, err := New(Config{QueueSize: 2, Concurrency: 1, ScriptRoots: []string{dir}, WorktreeRoots: []string{dir}, Scripts: map[string]string{"fake": script}})
	require.NoError(t, err)
	defer e.Close()
	j, err := e.Enqueue(Request{OrganizationID: "org-a", Wave: "W14e", Branch: "feat-worker", Brief: "brief", Worktree: dir, Script: "fake"})
	require.NoError(t, err)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, _ := e.Get(j.ID)
		if got.State == "running" {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	require.NoError(t, e.Stop(j.ID))
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, _ := e.Get(j.ID)
		if got.State == "cancelled" {
			return
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("worker was not cancelled")
}
func TestQueueSaturationAndPathValidation(t *testing.T) {
	e, err := New(Config{QueueSize: 1, Concurrency: 1, ScriptRoots: []string{"/tmp"}, WorktreeRoots: []string{"/tmp"}, Scripts: map[string]string{"fake": "/tmp/fake"}})
	require.NoError(t, err)
	defer e.Close()
	e.run = func(context.Context, Request) error { return nil }
	_, err = e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "x", Worktree: "/tmp", Script: "fake"})
	require.NoError(t, err)
	_, err = e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "x", Worktree: "/tmp", Script: "fake"})
	require.ErrorIs(t, err, ErrQueueFull)
	_, err = e.Enqueue(Request{OrganizationID: "other", Wave: "w", Branch: "b", Brief: "x", Worktree: "/etc", Script: "fake"})
	require.ErrorIs(t, err, ErrInvalid)
}
func TestInvalidCommandIsRejected(t *testing.T) {
	e, err := New(Config{ScriptRoots: []string{"/tmp"}, WorktreeRoots: []string{"/tmp"}, Scripts: map[string]string{"fake": "/tmp/fake"}})
	require.NoError(t, err)
	defer e.Close()
	_, err = e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "x", Worktree: "/tmp", Script: "sh -c evil"})
	require.ErrorIs(t, err, ErrInvalid)
}
