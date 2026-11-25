package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSetupPreservesIsIdempotentAndUninstalls(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".claude", "settings.local.json")
	original := `{"permissions":{"allow":["Read"]},"hooks":{"Stop":[{"matcher":"*","statusMessage":"keep me","hooks":[{"type":"command","command":"notify-user","timeout":17,"once":true}]}],"PreToolUse":[{"hooks":[{"type":"command","command":"audit"}]}]}}`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runSetup([]string{"claude", "--repository-id", "repo-a", "--wave", "W21f", "--worker", "worker"}); err != nil {
		t.Fatal(err)
	}
	status := exec.Command("git", "status", "--short")
	status.Dir = root
	if out, err := status.Output(); err != nil || len(out) != 0 {
		t.Fatalf("setup dirtied checkout: %q (%v)", out, err)
	}
	first := readJSON(t, path)
	if err := runSetup([]string{"claude", "--repository-id", "repo-a", "--wave", "W21f", "--worker", "worker"}); err != nil {
		t.Fatal(err)
	}
	second := readJSON(t, path)
	if string(second["permissions"]) != string(first["permissions"]) || string(second["hooks"]) != string(first["hooks"]) {
		t.Fatal("repeat setup changed semantic settings")
	}
	if !strings.Contains(string(second["hooks"]), `"async": true`) || !strings.Contains(string(second["hooks"]), "repo-a") {
		t.Fatal("installed hook missing async or repository metadata")
	}
	if !strings.Contains(string(second["hooks"]), `"timeout": 17`) || !strings.Contains(string(second["hooks"]), `"once": true`) || !strings.Contains(string(second["hooks"]), `"statusMessage": "keep me"`) {
		t.Fatal("install dropped optional Claude hook fields")
	}
	if err := runSetup([]string{"claude", "--uninstall"}); err != nil {
		t.Fatal(err)
	}
	removed := readJSON(t, path)
	if string(removed["permissions"]) != string(first["permissions"]) || strings.Contains(string(removed["hooks"]), "agentops hook") {
		t.Fatal("uninstall did not preserve unrelated configuration")
	}
	if !strings.Contains(string(removed["hooks"]), `"timeout": 17`) || !strings.Contains(string(removed["hooks"]), `"once": true`) || !strings.Contains(string(removed["hooks"]), `"statusMessage": "keep me"`) {
		t.Fatal("uninstall dropped optional Claude hook fields")
	}
	if err := runSetup([]string{"claude", "--uninstall"}); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeSetupBindsEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      string
		expected string
	}{
		{name: "explicit", args: []string{"--endpoint", "https://api.example.test/runs"}, expected: "https://api.example.test/runs"},
		{name: "environment", env: "https://env.example.test/runs", expected: "https://env.example.test/runs"},
		{name: "local default", expected: "http://127.0.0.1:8080/api/v1/agent-runs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			gitInit(t, root)
			old, _ := os.Getwd()
			t.Cleanup(func() { _ = os.Chdir(old) })
			if err := os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			if tc.env == "" {
				t.Setenv("AGENT_RUN_API_URL", "")
			} else {
				t.Setenv("AGENT_RUN_API_URL", tc.env)
			}
			args := append([]string{"claude", "--repository-id", "repo"}, tc.args...)
			if err := runSetup(args); err != nil {
				t.Fatal(err)
			}
			contents := string(mustRead(t, filepath.Join(root, ".claude/settings.local.json")))
			if !strings.Contains(contents, "--endpoint "+shellArg(tc.expected)) {
				t.Fatalf("hook does not bind endpoint %q: %s", tc.expected, contents)
			}
		})
	}
}

func TestClaudeSetupQuotesEndpoint(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	endpoint := "https://example.test/runs path; touch /tmp/agentops-should-not-exist && echo 'quoted'"
	if err := runSetup([]string{"claude", "--repository-id", "repo", "--endpoint", endpoint}); err != nil {
		t.Fatal(err)
	}
	contents := string(mustRead(t, filepath.Join(root, ".claude/settings.local.json")))
	encoded, err := json.Marshal("--endpoint " + shellArg(endpoint))
	if err != nil || !strings.Contains(contents, strings.Trim(string(encoded), `"`)) {
		t.Fatalf("hook endpoint is not safely quoted: %s", contents)
	}
}

func TestClaudeSetupSeparatesCheckouts(t *testing.T) {
	one, two := t.TempDir(), t.TempDir()
	for _, root := range []string{one, two} {
		gitInit(t, root)
	}
	for _, tc := range []struct{ root, repo string }{{one, "repo-one"}, {two, "repo-two"}} {
		old, _ := os.Getwd()
		if err := os.Chdir(tc.root); err != nil {
			t.Fatal(err)
		}
		if err := runSetup([]string{"claude", "--repository-id", tc.repo}); err != nil {
			t.Fatal(err)
		}
		_ = os.Chdir(old)
	}
	if strings.Contains(string(mustRead(t, filepath.Join(one, ".claude/settings.local.json"))), "repo-two") || !strings.Contains(string(mustRead(t, filepath.Join(two, ".claude/settings.local.json"))), "repo-two") {
		t.Fatal("checkout hook metadata crossed repositories")
	}
}

func TestClaudeSetupWorksInLinkedWorktree(t *testing.T) {
	mainRoot, linkedRoot := t.TempDir(), filepath.Join(t.TempDir(), "linked")
	gitInit(t, mainRoot)
	if err := os.WriteFile(filepath.Join(mainRoot, "tracked"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commit := exec.Command("git", "-C", mainRoot, "add", "tracked")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	commit = exec.Command("git", "-C", mainRoot, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "initial")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	cmd := exec.Command("git", "-C", mainRoot, "worktree", "add", "-q", "-b", "linked-test", linkedRoot)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("git", "-C", mainRoot, "worktree", "remove", "--force", linkedRoot).Run() })
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(linkedRoot); err != nil {
		t.Fatal(err)
	}
	if err := runSetup([]string{"claude", "--repository-id", "linked-repo"}); err != nil {
		t.Fatal(err)
	}
	status := exec.Command("git", "status", "--short")
	status.Dir = linkedRoot
	if out, err := status.Output(); err != nil || len(out) != 0 {
		t.Fatalf("linked worktree setup dirtied checkout: %q (%v)", out, err)
	}
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}
func readJSON(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	var v map[string]json.RawMessage
	if err := json.Unmarshal(mustRead(t, path), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
