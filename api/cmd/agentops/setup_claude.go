package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const claudeHookPrefix = "agentops hook"

type claudeHook struct {
	Matcher string          `json:"matcher"`
	Hooks   []claudeCommand `json:"hooks"`
}
type claudeCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async,omitempty"`
}

func runSetup(args []string) error {
	if len(args) < 1 || args[0] != "claude" {
		return errors.New("usage: agentops setup claude [--endpoint <url>] [--uninstall]")
	}
	fs := flag.NewFlagSet("setup claude", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	repository := fs.String("repository-id", os.Getenv("AGENT_RUN_REPOSITORY_ID"), "repository UUID")
	wave := fs.String("wave", "claude", "wave metadata")
	worker := fs.String("worker", "claude-hook", "worker metadata")
	endpoint := fs.String("endpoint", env("AGENT_RUN_API_URL", "http://127.0.0.1:8080/api/v1/agent-runs"), "agent-run API URL")
	uninstall := fs.Bool("uninstall", false, "remove the Agent Ops Claude hook")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("invalid setup flags")
	}
	root, err := gitRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, ".claude", "settings.local.json")
	if *uninstall {
		return editClaudeSettings(path, nil)
	}
	if strings.TrimSpace(*repository) == "" {
		return errors.New("--repository-id or AGENT_RUN_REPOSITORY_ID is required")
	}
	command := fmt.Sprintf("agentops hook --endpoint %s --repository-id %s --wave %s --worker %s", shellArg(*endpoint), shellArg(*repository), shellArg(*wave), shellArg(*worker))
	if err := editClaudeSettings(path, &claudeHook{Matcher: "", Hooks: []claudeCommand{{Type: "command", Command: command, Async: true}}}); err != nil {
		return err
	}
	return ignoreLocalClaudeSettings(root)
}

func gitRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", errors.New("run setup from a Git checkout")
	}
	return strings.TrimSpace(string(out)), nil
}

func editClaudeSettings(path string, install *claudeHook) error {
	settings := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("read Claude settings: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read Claude settings: %w", err)
	}
	hooks := map[string][]map[string]json.RawMessage{}
	if raw, ok := settings["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return fmt.Errorf("read Claude hooks: %w", err)
		}
	}
	stop := hooks["Stop"]
	filtered := make([]map[string]json.RawMessage, 0, len(stop))
	for _, group := range stop {
		rawCommands, ok := group["hooks"]
		if !ok {
			filtered = append(filtered, group)
			continue
		}
		var commands []map[string]json.RawMessage
		if err := json.Unmarshal(rawCommands, &commands); err != nil {
			return fmt.Errorf("read Claude Stop hooks: %w", err)
		}
		kept := make([]map[string]json.RawMessage, 0, len(commands))
		for _, hook := range commands {
			var command string
			_ = json.Unmarshal(hook["command"], &command)
			if !strings.HasPrefix(command, claudeHookPrefix+" ") {
				kept = append(kept, hook)
			}
		}
		if len(kept) > 0 {
			encoded, err := json.Marshal(kept)
			if err != nil {
				return err
			}
			group["hooks"] = encoded
			filtered = append(filtered, group)
		}
	}
	if install != nil {
		encoded, err := json.Marshal(install)
		if err != nil {
			return err
		}
		var group map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &group); err != nil {
			return err
		}
		filtered = append(filtered, group)
	}
	if len(filtered) == 0 {
		delete(hooks, "Stop")
	} else {
		hooks["Stop"] = filtered
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		raw, err := json.Marshal(hooks)
		if err != nil {
			return err
		}
		settings["hooks"] = raw
	}
	if len(settings) == 0 {
		return removeSettings(path)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func removeSettings(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ignoreLocalClaudeSettings(root string) error {
	cmd := exec.Command("git", "rev-parse", "--git-path", "info/exclude")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return errors.New("resolve Git exclude path")
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Git exclude: %w", err)
	}
	const entry = ".claude/settings.local.json"
	if strings.Contains("\n"+string(data)+"\n", "\n"+entry+"\n") {
		return nil
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, []byte(entry+"\n")...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func shellArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
