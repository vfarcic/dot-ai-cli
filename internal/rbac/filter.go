package rbac

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/config"
)

const toolPathPrefix = "/api/v1/tools/"

// toolsResponse matches the GET /api/v1/tools response schema.
type toolsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	} `json:"data"`
}

// FilterCommands fetches the user's allowed tools (if OAuth) and hides
// disallowed tool commands from the cobra tree. If the fetch fails, all
// commands remain visible (graceful degradation).
func FilterCommands(root *cobra.Command, cfg *config.Config) {
	if cfg.TokenSource != config.TokenSourceOAuth {
		return
	}

	allowed, err := fetchAllowedTools(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch tool permissions: %v\n", err)
		return
	}

	for _, cmd := range root.Commands() {
		path := cmd.Annotations["path"]
		if path == "" || !strings.HasPrefix(path, toolPathPrefix) {
			continue
		}

		toolName := extractToolName(path)
		if toolName == "" {
			continue
		}

		if !allowed[toolName] {
			hideCommand(cmd, toolName)
		}
	}
}

// fetchAllowedTools calls GET /api/v1/tools and returns a set of allowed
// tool names.
func fetchAllowedTools(cfg *config.Config) (map[string]bool, error) {
	body, err := client.Do(cfg, "GET", "/api/v1/tools", nil)
	if err != nil {
		return nil, err
	}

	var resp toolsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse tools response: %w", err)
	}

	allowed := make(map[string]bool, len(resp.Data.Tools))
	for _, t := range resp.Data.Tools {
		allowed[t.Name] = true
	}
	return allowed, nil
}

// extractToolName returns the tool name from a path like
// "/api/v1/tools/query" → "query".
func extractToolName(path string) string {
	rest := strings.TrimPrefix(path, toolPathPrefix)
	if rest == "" {
		return ""
	}
	// Handle paths with subresources: take only the first segment.
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

// hideCommand marks a command as hidden and replaces its RunE with a
// permission-denied error.
func hideCommand(cmd *cobra.Command, toolName string) {
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return &client.RequestError{
			Message:  fmt.Sprintf("you do not have permission to use the '%s' command. Contact your administrator to request access.", toolName),
			ExitCode: client.ExitToolError,
		}
	}
}
