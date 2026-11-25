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
	original := `{"permissions":{"allow":["Read"]},"hooks":{"Stop":[{"matcher":"*","hooks":[{"type":"command","command":"notify-user"}]}],"PreToolUse":[{"hooks":[{"type":"command","command":"audit"}]}]}}`
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
	if err := runSetup([]string{"claude", "--uninstall"}); err != nil {
		t.Fatal(err)
	}
	removed := readJSON(t, path)
	if string(removed["permissions"]) != string(first["permissions"]) || strings.Contains(string(removed["hooks"]), "agentops hook") {
		t.Fatal("uninstall did not preserve unrelated configuration")
	}
	if err := runSetup([]string{"claude", "--uninstall"}); err != nil {
		t.Fatal(err)
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
