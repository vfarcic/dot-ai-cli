package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vfarcic/dot-ai-cli/internal/client"
)

const (
	settingsFile     = ".claude/settings.json"
	hookCommandBase  = "dot-ai skills generate --agent claude-code"
	hookCommandPrefix = "dot-ai skills generate"
	hookMatcher      = "startup"
	hookType         = "command"
	hookEventKey     = "SessionStart"
)

// BuildHookCommand constructs the hook command string from the resolved flags,
// mirroring the arguments passed to `skills generate`.
func BuildHookCommand(include, exclude string, customOnly bool) string {
	cmd := hookCommandBase
	if customOnly {
		cmd += " --custom-only"
	}
	if include != "" {
		cmd += fmt.Sprintf(" --include %q", include)
	}
	if exclude != "" {
		cmd += fmt.Sprintf(" --exclude %q", exclude)
	}
	return cmd
}

// InstallSessionHook writes a Claude Code SessionStart hook into
// .claude/settings.json that re-runs skills generation on session startup.
// It merges with existing settings and is idempotent. If an existing
// dot-ai skills generate hook exists with different flags, it is replaced.
func InstallSessionHook(command string) error {
	settings, err := readSettings(settingsFile)
	if err != nil {
		return err
	}

	if hookExact(settings, command) {
		return nil
	}

	removeExistingHook(settings)
	insertHook(settings, command)

	return writeSettings(settingsFile, settings)
}

func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to read %s: %v", path, err),
			ExitCode: client.ExitToolError,
		}
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to parse %s: %v", path, err),
			ExitCode: client.ExitToolError,
		}
	}
	return settings, nil
}

func writeSettings(path string, settings map[string]any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to create directory %s: %v", dir, err),
			ExitCode: client.ExitToolError,
		}
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to marshal settings: %v", err),
			ExitCode: client.ExitToolError,
		}
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to write %s: %v", path, err),
			ExitCode: client.ExitToolError,
		}
	}
	return nil
}

// hookExact returns true if the settings already contain a hook with the exact command.
func hookExact(settings map[string]any, command string) bool {
	for _, cmd := range findHookCommands(settings) {
		if cmd == command {
			return true
		}
	}
	return false
}

// removeExistingHook removes any existing dot-ai skills generate hook so it
// can be replaced with an updated command.
func removeExistingHook(settings map[string]any) {
	hooks, ok := settings["hooks"]
	if !ok {
		return
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		return
	}
	sessionStart, ok := hooksMap[hookEventKey]
	if !ok {
		return
	}
	entries, ok := sessionStart.([]any)
	if !ok {
		return
	}

	var kept []any
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			kept = append(kept, entry)
			continue
		}
		matcher, _ := entryMap["matcher"].(string)
		if matcher != hookMatcher {
			kept = append(kept, entry)
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok {
			kept = append(kept, entry)
			continue
		}
		var keptInner []any
		for _, h := range innerHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				keptInner = append(keptInner, h)
				continue
			}
			cmd, _ := hMap["command"].(string)
			if hMap["type"] == hookType && strings.HasPrefix(cmd, hookCommandPrefix) {
				continue // remove this hook
			}
			keptInner = append(keptInner, h)
		}
		if len(keptInner) > 0 {
			entryMap["hooks"] = keptInner
			kept = append(kept, entry)
		}
	}
	hooksMap[hookEventKey] = kept
}

// findHookCommands returns all dot-ai skills generate commands found in the settings.
func findHookCommands(settings map[string]any) []string {
	hooks, ok := settings["hooks"]
	if !ok {
		return nil
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		return nil
	}
	sessionStart, ok := hooksMap[hookEventKey]
	if !ok {
		return nil
	}
	entries, ok := sessionStart.([]any)
	if !ok {
		return nil
	}

	var cmds []string
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		matcher, _ := entryMap["matcher"].(string)
		if matcher != hookMatcher {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hMap["command"].(string)
			if hMap["type"] == hookType && strings.HasPrefix(cmd, hookCommandPrefix) {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

func insertHook(settings map[string]any, command string) {
	hookEntry := map[string]any{
		"matcher": hookMatcher,
		"hooks": []any{
			map[string]any{
				"type":    hookType,
				"command": command,
			},
		},
	}

	hooks, ok := settings["hooks"]
	if !ok {
		settings["hooks"] = map[string]any{
			hookEventKey: []any{hookEntry},
		}
		return
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		settings["hooks"] = map[string]any{
			hookEventKey: []any{hookEntry},
		}
		return
	}

	sessionStart, ok := hooksMap[hookEventKey]
	if !ok {
		hooksMap[hookEventKey] = []any{hookEntry}
		return
	}
	entries, ok := sessionStart.([]any)
	if !ok {
		hooksMap[hookEventKey] = []any{hookEntry}
		return
	}

	hooksMap[hookEventKey] = append(entries, hookEntry)
}
