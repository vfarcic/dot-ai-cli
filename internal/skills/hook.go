package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vfarcic/dot-ai-cli/internal/client"
)

const (
	settingsFile = ".claude/settings.json"
	hookCommand  = "dot-ai skills generate --agent claude-code"
	hookMatcher  = "startup"
	hookType     = "command"
	hookEventKey = "SessionStart"
)

// InstallSessionHook writes a Claude Code SessionStart hook into
// .claude/settings.json that re-runs skills generation on session startup.
// It merges with existing settings and is idempotent.
func InstallSessionHook() error {
	settings, err := readSettings(settingsFile)
	if err != nil {
		return err
	}

	if hookExists(settings) {
		return nil
	}

	insertHook(settings)

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

func hookExists(settings map[string]any) bool {
	hooks, ok := settings["hooks"]
	if !ok {
		return false
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		return false
	}
	sessionStart, ok := hooksMap[hookEventKey]
	if !ok {
		return false
	}
	entries, ok := sessionStart.([]any)
	if !ok {
		return false
	}

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
			if hMap["type"] == hookType && hMap["command"] == hookCommand {
				return true
			}
		}
	}
	return false
}

func insertHook(settings map[string]any) {
	hookEntry := map[string]any{
		"matcher": hookMatcher,
		"hooks": []any{
			map[string]any{
				"type":    hookType,
				"command": hookCommand,
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
