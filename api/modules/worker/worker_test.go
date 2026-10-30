package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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

func TestEnqueueReturnsStableSnapshotWhileJobExecutes(t *testing.T) {
	e, err := New(Config{QueueSize: 1, Concurrency: 1, ScriptRoots: []string{"/tmp"}, WorktreeRoots: []string{"/tmp"}, Scripts: map[string]string{"fake": "/tmp/fake"}})
	require.NoError(t, err)
	defer e.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	e.run = func(context.Context, Request) error {
		close(started)
		<-release
		return nil
	}

	job, err := e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "brief", Worktree: "/tmp", Script: "fake"})
	require.NoError(t, err)
	<-started
	for i := 0; i < 1000; i++ {
		require.Equal(t, "queued", job.State)
	}
	close(release)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stored, getErr := e.Get(job.ID)
		require.NoError(t, getErr)
		if stored.State == "succeeded" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("worker did not finish")
}

type testMemberships struct{ role string }

type auditSink struct{ entries []*ActionAudit }

func (s *auditSink) Create(_ context.Context, audit *ActionAudit) error {
	s.entries = append(s.entries, audit)
	return nil
}

func (m testMemberships) Membership(context.Context, string, string) (*organization.Membership, error) {
	return &organization.Membership{Role: m.role}, nil
}

func TestControllerWorkerAccessIsOrganizationScoped(t *testing.T) {
	e, err := New(Config{QueueSize: 2, ScriptRoots: []string{"/tmp"}, WorktreeRoots: []string{"/tmp"}, Scripts: map[string]string{"fake": "/tmp/fake"}})
	require.NoError(t, err)
	defer e.Close()
	j, err := e.Enqueue(Request{OrganizationID: "org-a", Wave: "w", Branch: "b", Brief: "x", Worktree: "/tmp", Script: "fake"})
	require.NoError(t, err)
	c := NewController(e, testMemberships{role: organization.RoleOwner})
	for _, action := range []string{"get", "stop"} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("GET", "/workers/"+j.ID, nil)
		ctx.Set("organization_id", "org-b")
		ctx.Set("account_id", "owner-b")
		ctx.Params = gin.Params{{Key: "worker_id", Value: j.ID}}
		if action == "get" {
			c.Get(ctx)
		} else {
			c.Stop(ctx)
		}
		require.Contains(t, []int{403, 404}, w.Code, "%s leaked cross-tenant access", action)
		got, getErr := e.Get(j.ID)
		require.NoError(t, getErr)
		require.NotEqual(t, "cancelled", got.State)
	}
}

func TestControllerRecordsAcceptedAndRejectedActions(t *testing.T) {
	e, err := New(Config{QueueSize: 1, Concurrency: 1, ScriptRoots: []string{"/tmp"}, WorktreeRoots: []string{"/tmp"}, Scripts: map[string]string{"fake": "/tmp/fake"}})
	require.NoError(t, err)
	defer e.Close()
	sink := &auditSink{}
	c := NewController(e, testMemberships{role: organization.RoleOwner}, sink)
	c.now = func() time.Time { return time.Date(2026, 10, 30, 1, 2, 3, 0, time.UTC) }
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/workers/start", strings.NewReader(`{"wave":"W14","branch":"feat/a","brief":"brief","worktree":"/tmp","script":"fake"}`))
	ctx.Set("organization_id", "org-a")
	ctx.Set("account_id", "actor-a")
	c.Start(ctx)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, sink.entries, 1)
	require.NotEmpty(t, sink.entries[0].ID)
	require.Equal(t, "org-a", sink.entries[0].OrganizationID)
	require.Equal(t, "actor-a", sink.entries[0].ActorID)
	require.Equal(t, "start", sink.entries[0].Action)
	require.Equal(t, "W14", sink.entries[0].Wave)
	require.NotEmpty(t, sink.entries[0].JobID)
	require.Equal(t, "accepted", sink.entries[0].Outcome)
	require.Equal(t, c.now(), sink.entries[0].OccurredAt)

	w = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/workers/start", strings.NewReader(`{"wave":"","branch":"bad","brief":""}`))
	ctx.Set("organization_id", "org-a")
	ctx.Set("account_id", "actor-a")
	c.Start(ctx)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "rejected", sink.entries[1].Outcome)
}

func TestCancelledQueuedJobIsSkippedAndSaturationIsDeterministic(t *testing.T) {
	e, err := New(Config{QueueSize: 1, Concurrency: 1, ScriptRoots: []string{"/tmp"}, WorktreeRoots: []string{"/tmp"}, Scripts: map[string]string{"fake": "/tmp/fake"}})
	require.NoError(t, err)
	defer e.Close()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var mu sync.Mutex
	called := 0
	e.run = func(context.Context, Request) error {
		mu.Lock()
		called++
		mu.Unlock()
		started <- struct{}{}
		<-release
		return nil
	}
	first, err := e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "first", Worktree: "/tmp", Script: "fake"})
	require.NoError(t, err)
	<-started
	second, err := e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "second", Worktree: "/tmp", Script: "fake"})
	require.NoError(t, err)
	_, err = e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "third", Worktree: "/tmp", Script: "fake"})
	require.ErrorIs(t, err, ErrQueueFull)
	require.NoError(t, e.Stop(second.ID))
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, _ := e.Get(first.ID)
		if got.State == "succeeded" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, called)
}
func TestQueueSaturationAndPathValidation(t *testing.T) {
	e, err := New(Config{QueueSize: 1, Concurrency: 1, ScriptRoots: []string{"/tmp"}, WorktreeRoots: []string{"/tmp"}, Scripts: map[string]string{"fake": "/tmp/fake"}})
	require.NoError(t, err)
	defer e.Close()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	e.run = func(context.Context, Request) error {
		started <- struct{}{}
		<-release
		return nil
	}
	_, err = e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "x", Worktree: "/tmp", Script: "fake"})
	require.NoError(t, err)
	<-started
	_, err = e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "x", Worktree: "/tmp", Script: "fake"})
	require.NoError(t, err)
	_, err = e.Enqueue(Request{OrganizationID: "org", Wave: "w", Branch: "b", Brief: "x", Worktree: "/tmp", Script: "fake"})
	require.ErrorIs(t, err, ErrQueueFull)
	close(release)
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
