package worker

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWorkerLogsStreamProcessOutputAndCompletion(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fixture.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf 'out-1\\n'; printf 'err-2\\n' >&2\nprintf 'out-3\\n'\n"), 0755))
	e, err := New(Config{ScriptRoots: []string{dir}, WorktreeRoots: []string{dir}, Scripts: map[string]string{"fixture": script}})
	require.NoError(t, err)
	defer e.Close()
	j, err := e.Enqueue(Request{OrganizationID: "org-a", Wave: "W14", Branch: "feat/logs", Brief: "brief", Worktree: dir, Script: "fixture"})
	require.NoError(t, err)
	history, lines, unsubscribe, err := e.SubscribeLogs(j.ID, "org-a")
	require.NoError(t, err)
	defer unsubscribe()
	var got []string
	got = append(got, history...)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line, open := <-lines:
			if !open {
				require.Equal(t, []string{"out-1", "err-2", "out-3"}, got)
				return
			}
			got = append(got, line)
		case <-deadline:
			t.Fatal("log stream did not complete")
		}
	}
}

func TestWorkerLogsRejectsForeignTenantAndRoleBeforeUpgrade(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fixture.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nsleep 1\n"), 0755))
	e, err := New(Config{ScriptRoots: []string{dir}, WorktreeRoots: []string{dir}, Scripts: map[string]string{"fixture": script}})
	require.NoError(t, err)
	defer e.Close()
	j, err := e.Enqueue(Request{OrganizationID: "org-a", Wave: "W14", Branch: "feat/logs", Brief: "brief", Worktree: dir, Script: "fixture"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, org, role string
		status          int
	}{{"foreign", "org-b", organization.RoleOwner, 404}, {"member", "org-a", organization.RoleMember, 403}} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			c := NewController(e, testMemberships{role: tc.role})
			r.Use(func(ctx *gin.Context) { ctx.Set("organization_id", tc.org); ctx.Set("account_id", "actor"); ctx.Next() })
			NewRoutes(c).Register(r.Group("/api/v1"))
			s := httptest.NewServer(r)
			defer s.Close()
			conn, resp, err := websocket.DefaultDialer.Dial("ws"+s.URL[len("http"):]+"/api/v1/workers/"+j.ID+"/logs", nil)
			if conn != nil {
				require.NoError(t, conn.Close())
			}
			require.Error(t, err)
			require.Equal(t, tc.status, resp.StatusCode)
		})
	}
}

func TestWorkerLogsSlowSubscriberDoesNotBlockProcess(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fixture.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ni=0; while [ $i -lt 1000 ]; do printf 'line-%s\\n' $i; i=$((i+1)); done\n"), 0755))
	e, err := New(Config{ScriptRoots: []string{dir}, WorktreeRoots: []string{dir}, Scripts: map[string]string{"fixture": script}})
	require.NoError(t, err)
	defer e.Close()
	j, err := e.Enqueue(Request{OrganizationID: "org-a", Wave: "W14", Branch: "feat/logs", Brief: "brief", Worktree: dir, Script: "fixture"})
	require.NoError(t, err)
	_, _, unsubscribe, err := e.SubscribeLogs(j.ID, "org-a")
	require.NoError(t, err)
	defer unsubscribe()
	waitForJob(t, e, j.ID, "succeeded")
}
