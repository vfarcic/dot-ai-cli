package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/config"
)

// AgentDirs maps agent names to their skills directory paths.
var AgentDirs = map[string]string{
	"claude-code": ".claude/skills",
	"cursor":      ".cursor/skills",
	"windsurf":    ".windsurf/skills",
}

// toolsResponse matches the GET /api/v1/tools response schema.
type toolsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Tools []toolDef `json:"tools"`
	} `json:"data"`
}

type toolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  []paramDef `json:"parameters"`
}

type paramDef struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum"`
}

// promptsResponse matches the GET /api/v1/prompts response schema.
type promptsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Prompts []promptDef `json:"prompts"`
	} `json:"data"`
}

type promptDef struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Arguments   []argDef  `json:"arguments"`
}

type argDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// promptRenderResponse matches the POST /api/v1/prompts/{name} response.
type promptRenderResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Description string       `json:"description"`
		Messages    []promptMsg  `json:"messages"`
	} `json:"data"`
}

type promptMsg struct {
	Role    string `json:"role"`
	Content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// Generate fetches tools and prompts from the server and writes SKILL.md
// files to the resolved output directory. Returns the output directory used.
// routingSkill is the embedded routing skill content to write as dot-ai/SKILL.md.
func Generate(cfg *config.Config, agent, path string, routingSkill []byte) (string, error) {
	outDir, err := resolveDir(agent, path)
	if err != nil {
		return "", err
	}

	tools, err := fetchTools(cfg)
	if err != nil {
		return "", err
	}

	prompts, err := fetchPrompts(cfg)
	if err != nil {
		return "", err
	}

	if err := cleanExisting(outDir); err != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to clean existing skills: %v", err),
			ExitCode: client.ExitToolError,
		}
	}

	for _, t := range tools {
		if err := writeToolSkill(outDir, t); err != nil {
			return "", &client.RequestError{
				Message:  fmt.Sprintf("Error: failed to write skill for tool %q: %v", t.Name, err),
				ExitCode: client.ExitToolError,
			}
		}
	}

	for _, p := range prompts {
		rendered := renderPrompt(cfg, p.Name)
		if err := writePromptSkill(outDir, p, rendered); err != nil {
			return "", &client.RequestError{
				Message:  fmt.Sprintf("Error: failed to write skill for prompt %q: %v", p.Name, err),
				ExitCode: client.ExitToolError,
			}
		}
	}

	if err := writeRoutingSkill(outDir, routingSkill); err != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to write routing skill: %v", err),
			ExitCode: client.ExitToolError,
		}
	}

	return outDir, nil
}

func resolveDir(agent, path string) (string, error) {
	if path != "" {
		return path, nil
	}
	dir, ok := AgentDirs[agent]
	if !ok {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: unknown agent %q. Supported: claude-code, cursor, windsurf", agent),
			ExitCode: client.ExitUsageError,
		}
	}
	return dir, nil
}

func fetchTools(cfg *config.Config) ([]toolDef, error) {
	body, err := client.Do(cfg, "GET", "/api/v1/tools", nil)
	if err != nil {
		return nil, err
	}
	var resp toolsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to parse tools response: %v", err),
			ExitCode: client.ExitToolError,
		}
	}
	return resp.Data.Tools, nil
}

func fetchPrompts(cfg *config.Config) ([]promptDef, error) {
	body, err := client.Do(cfg, "GET", "/api/v1/prompts", nil)
	if err != nil {
		return nil, err
	}
	var resp promptsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to parse prompts response: %v", err),
			ExitCode: client.ExitToolError,
		}
	}
	return resp.Data.Prompts, nil
}

// renderPrompt attempts to fetch the rendered content of a prompt.
// Returns nil if the render fails (e.g., required arguments missing).
func renderPrompt(cfg *config.Config, name string) *promptRenderResponse {
	body, err := client.Do(cfg, "POST", "/api/v1/prompts/"+name, nil)
	if err != nil {
		return nil
	}
	var resp promptRenderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	return &resp
}

// cleanExisting removes the dot-ai routing skill and all dot-ai-* directories
// from the output path.
func cleanExisting(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && (e.Name() == "dot-ai" || strings.HasPrefix(e.Name(), "dot-ai-")) {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeToolSkill(dir string, t toolDef) error {
	skillDir := filepath.Join(dir, "dot-ai-"+t.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: dot-ai-%s\n", t.Name))
	b.WriteString(fmt.Sprintf("description: %s\n", yamlEscape(t.Description)))
	b.WriteString("user-invocable: true\n")
	b.WriteString("---\n\n")

	b.WriteString(fmt.Sprintf("# dot-ai %s\n\n", t.Name))
	if t.Description != "" {
		b.WriteString(t.Description + "\n\n")
	}

	b.WriteString("## Usage\n\n")
	b.WriteString(fmt.Sprintf("```bash\ndot-ai %s", t.Name))

	// Build usage line showing positional and flag params.
	var positional, flags []paramDef
	for _, p := range t.Parameters {
		if p.Required && p.Type == "string" && len(p.Enum) == 0 {
			positional = append(positional, p)
		} else {
			flags = append(flags, p)
		}
	}
	// Only promote if there's exactly one candidate (same logic as dynamic.go).
	if len(positional) != 1 {
		flags = t.Parameters
		positional = nil
	}
	for _, p := range positional {
		b.WriteString(fmt.Sprintf(" <%s>", p.Name))
	}
	if len(flags) > 0 {
		b.WriteString(" [flags]")
	}
	b.WriteString("\n```\n")

	if len(t.Parameters) > 0 {
		b.WriteString("\n## Parameters\n\n")
		for _, p := range t.Parameters {
			req := "optional"
			if p.Required {
				req = "required"
			}
			desc := p.Description
			if len(p.Enum) > 0 {
				desc += fmt.Sprintf(" (one of: %s)", strings.Join(p.Enum, ", "))
			}
			b.WriteString(fmt.Sprintf("- `--%s` (%s, %s): %s\n", p.Name, p.Type, req, desc))
		}
	}

	return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(b.String()), 0o644)
}

func writePromptSkill(dir string, p promptDef, rendered *promptRenderResponse) error {
	skillDir := filepath.Join(dir, "dot-ai-"+p.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: dot-ai-%s\n", p.Name))
	desc := p.Description
	if desc == "" {
		desc = p.Name + " prompt"
	}
	b.WriteString(fmt.Sprintf("description: %s\n", yamlEscape(desc)))
	b.WriteString("user-invocable: true\n")
	b.WriteString("---\n\n")

	// If we have rendered content, use the prompt messages directly.
	if rendered != nil && len(rendered.Data.Messages) > 0 {
		for _, msg := range rendered.Data.Messages {
			b.WriteString(msg.Content.Text)
			b.WriteString("\n\n")
		}
	} else {
		// Fallback: document the prompt metadata.
		b.WriteString(fmt.Sprintf("# %s\n\n", p.Name))
		if p.Description != "" {
			b.WriteString(p.Description + "\n\n")
		}
		if len(p.Arguments) > 0 {
			b.WriteString("## Arguments\n\n")
			for _, a := range p.Arguments {
				req := "optional"
				if a.Required {
					req = "required"
				}
				desc := a.Description
				if desc == "" {
					desc = a.Name
				}
				b.WriteString(fmt.Sprintf("- `%s` (%s): %s\n", a.Name, req, desc))
			}
		}
	}

	return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(b.String()), 0o644)
}

func writeRoutingSkill(dir string, content []byte) error {
	skillDir := filepath.Join(dir, "dot-ai")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0o644)
}

// yamlEscape wraps a string in quotes if it contains YAML-special characters.
func yamlEscape(s string) string {
	if strings.ContainsAny(s, ":#{}[]|>&*!%@`\"'\n") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
