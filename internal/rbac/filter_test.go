package rbac

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/config"
)

// newToolsServer returns an httptest server that responds to GET /api/v1/tools
// with the given tool names.
func newToolsServer(t *testing.T, toolNames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tools" {
			http.NotFound(w, r)
			return
		}
		type tool struct {
			Name string `json:"name"`
		}
		var tools []tool
		for _, n := range toolNames {
			tools = append(tools, tool{Name: n})
		}
		resp := map[string]any{
			"success": true,
			"data": map[string]any{
				"tools": tools,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// buildTestRoot creates a root command with tool subcommands for testing.
func buildTestRoot(toolNames []string) *cobra.Command {
	root := &cobra.Command{Use: "dot-ai"}
	for _, name := range toolNames {
		cmd := &cobra.Command{
			Use:   name,
			Short: name + " tool",
			Annotations: map[string]string{
				"path": "/api/v1/tools/" + name,
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				return nil
			},
		}
		root.AddCommand(cmd)
	}
	// Add a non-tool command.
	root.AddCommand(&cobra.Command{
		Use:   "resources",
		Short: "List resources",
		Annotations: map[string]string{
			"path": "/api/v1/resources",
		},
	})
	return root
}

func TestFilterCommands_BlocksDisallowedTools(t *testing.T) {
	srv := newToolsServer(t, []string{"query", "recommend"})
	defer srv.Close()

	root := buildTestRoot([]string{"query", "recommend", "operate", "version"})
	cfg := &config.Config{
		ServerURL:   srv.URL,
		Token:       "test-oauth-token",
		TokenSource: config.TokenSourceOAuth,
	}

	FilterCommands(root, cfg)

	// Allowed commands should execute without error.
	for _, name := range []string{"query", "recommend"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Errorf("expected allowed command %q to be findable", name)
			continue
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("expected allowed command %q to succeed, got: %s", name, err)
		}
	}

	// Disallowed commands should return permission error.
	for _, name := range []string{"operate", "version"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Errorf("expected disallowed command %q to still be findable", name)
			continue
		}
		if err := cmd.RunE(cmd, nil); err == nil {
			t.Errorf("expected disallowed command %q to return error", name)
		} else if !strings.Contains(err.Error(), "permission") {
			t.Errorf("expected permission error for %q, got: %s", name, err.Error())
		}
	}

	// Non-tool command should not be affected.
	cmd, _, _ := root.Find([]string{"resources"})
	if cmd.RunE != nil {
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("expected non-tool command 'resources' to not be blocked, got: %s", err)
		}
	}
}

func TestFilterCommands_SkipsNonOAuth(t *testing.T) {
	root := buildTestRoot([]string{"query", "operate"})

	// Static token — no filtering should happen.
	cfg := &config.Config{
		ServerURL:   "http://unreachable:9999",
		Token:       "static-token",
		TokenSource: config.TokenSourceStatic,
	}
	FilterCommands(root, cfg)

	for _, name := range []string{"query", "operate"} {
		cmd, _, _ := root.Find([]string{name})
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("expected command %q to not be blocked with static token, got: %s", name, err)
		}
	}
}

func TestFilterCommands_SkipsUnauthenticated(t *testing.T) {
	root := buildTestRoot([]string{"query", "operate"})

	cfg := &config.Config{
		ServerURL:   "http://unreachable:9999",
		TokenSource: config.TokenSourceNone,
	}
	FilterCommands(root, cfg)

	for _, name := range []string{"query", "operate"} {
		cmd, _, _ := root.Find([]string{name})
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("expected command %q to not be blocked when unauthenticated, got: %s", name, err)
		}
	}
}

func TestFilterCommands_GracefulDegradation(t *testing.T) {
	root := buildTestRoot([]string{"query", "operate"})

	// Unreachable server — should not block anything.
	cfg := &config.Config{
		ServerURL:   "http://localhost:19999",
		Token:       "oauth-token",
		TokenSource: config.TokenSourceOAuth,
	}
	FilterCommands(root, cfg)

	for _, name := range []string{"query", "operate"} {
		cmd, _, _ := root.Find([]string{name})
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("expected command %q to not be blocked on fetch failure, got: %s", name, err)
		}
	}
}
