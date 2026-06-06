package skills

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/config"
	"github.com/vfarcic/dot-ai-cli/internal/openapi"
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
		Source  string      `json:"source"`
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

// promptFile represents a supporting file returned by the server, with
// base64-encoded content.
type promptFile struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64-encoded
}

// promptRenderResponse matches the POST /api/v1/prompts/{name} response.
type promptRenderResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Description string       `json:"description"`
		Messages    []promptMsg  `json:"messages"`
		Files       []promptFile `json:"files"`
	} `json:"data"`
}

type promptMsg struct {
	Role    string `json:"role"`
	Content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// RefreshPrompts asks the server to pull the latest prompts from the
// configured git repository, busting the server-side cache.
func RefreshPrompts(cfg *config.Config) error {
	_, err := client.Do(cfg, "POST", "/api/v1/prompts/refresh", nil)
	return err
}

// filterByName applies include/exclude regex patterns to a list of names.
// Include is applied first (keep only matching), then exclude (remove matching).
// Empty patterns mean no filtering at that stage.
func filterByName(names []string, include, exclude string) ([]string, error) {
	var includeRe, excludeRe *regexp.Regexp
	if include != "" {
		var err error
		includeRe, err = regexp.Compile(include)
		if err != nil {
			return nil, &client.RequestError{
				Message:  fmt.Sprintf("Error: invalid include pattern %q: %v", include, err),
				ExitCode: client.ExitUsageError,
			}
		}
	}
	if exclude != "" {
		var err error
		excludeRe, err = regexp.Compile(exclude)
		if err != nil {
			return nil, &client.RequestError{
				Message:  fmt.Sprintf("Error: invalid exclude pattern %q: %v", exclude, err),
				ExitCode: client.ExitUsageError,
			}
		}
	}
	var filtered []string
	for _, name := range names {
		if includeRe != nil && !includeRe.MatchString(name) {
			continue
		}
		if excludeRe != nil && excludeRe.MatchString(name) {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered, nil
}

// Generate fetches tools and prompts from the server and writes SKILL.md
// files to the resolved output directory. Returns the output directory and
// the source identifier returned by the server (e.g. "built-in" or the
// supplied repo URL). include and exclude are optional regex patterns for
// filtering skills by name (without the dot-ai- prefix). routingSkill is the
// embedded routing skill content to write as dot-ai/SKILL.md. When repo is
// non-empty, it is passed through to the server as ?repo=<url> on the prompts
// list and render calls, overriding the server's configured default repo.
//
// Per PRD #12, Generate performs a per-source wipe-and-replace: only skills
// whose `source:` frontmatter matches the current invocation's source are
// removed before the fresh set is written. Skills from other sources are left
// untouched. Cross-source name collisions are resolved first-source-wins with
// a warning to stderr. An exclusive file lock on <outDir>/.dot-ai.lock
// serializes concurrent invocations.
func Generate(cfg *config.Config, agent, path, include, exclude string, customOnly bool, routingSkill []byte, repo string) (string, string, error) {
	outDir, err := resolveDir(agent, path)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to create output directory %s: %v", outDir, err),
			ExitCode: client.ExitToolError,
		}
	}

	lock, err := acquireLock(outDir)
	if err != nil {
		return "", "", &client.RequestError{
			Message:  fmt.Sprintf("Error: %v", err),
			ExitCode: client.ExitToolError,
		}
	}
	defer lock.Release()

	var tools []toolDef
	if !customOnly {
		tools, err = fetchTools(cfg)
		if err != nil {
			return "", "", err
		}
	}

	prompts, source, err := fetchPrompts(cfg, repo)
	if err != nil {
		return "", "", err
	}

	if include != "" || exclude != "" {
		toolNames := make([]string, len(tools))
		for i, t := range tools {
			toolNames[i] = t.Name
		}
		kept, err := filterByName(toolNames, include, exclude)
		if err != nil {
			return "", "", err
		}
		keptSet := make(map[string]bool, len(kept))
		for _, n := range kept {
			keptSet[n] = true
		}
		filtered := tools[:0]
		for _, t := range tools {
			if keptSet[t.Name] {
				filtered = append(filtered, t)
			}
		}
		tools = filtered

		promptNames := make([]string, len(prompts))
		for i, p := range prompts {
			promptNames[i] = p.Name
		}
		kept, err = filterByName(promptNames, include, exclude)
		if err != nil {
			return "", "", err
		}
		keptSet = make(map[string]bool, len(kept))
		for _, n := range kept {
			keptSet[n] = true
		}
		filteredPrompts := prompts[:0]
		for _, p := range prompts {
			if keptSet[p.Name] {
				filteredPrompts = append(filteredPrompts, p)
			}
		}
		prompts = filteredPrompts
	}

	existing, err := scanExistingSkills(outDir)
	if err != nil {
		return "", "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to scan existing skills: %v", err),
			ExitCode: client.ExitToolError,
		}
	}

	// Per-source wipe. Untagged legacy files are treated as belonging to the
	// env-var repo (current source) only when --repo was NOT supplied; when
	// the caller explicitly passed --repo, legacy files survive (they belong
	// to a different source the caller hasn't claimed yet).
	wipeUntagged := repo == ""
	for name, sk := range existing {
		if sk.Source == source || (wipeUntagged && sk.Source == "") {
			if err := os.RemoveAll(sk.Path); err != nil {
				return "", "", &client.RequestError{
					Message:  fmt.Sprintf("Error: failed to clean existing skill %q: %v", name, err),
					ExitCode: client.ExitToolError,
				}
			}
			delete(existing, name)
		}
	}

	for _, t := range tools {
		if other, ok := existing["dot-ai-"+t.Name]; ok && other.Source != "" && other.Source != source {
			fmt.Fprintf(os.Stderr, "warning: skipping %q: already provided by source %q (first-source-wins)\n", t.Name, RedactURL(other.Source))
			continue
		}
		if err := writeToolSkill(outDir, t, source); err != nil {
			return "", "", &client.RequestError{
				Message:  fmt.Sprintf("Error: failed to write skill for tool %q: %v", t.Name, err),
				ExitCode: client.ExitToolError,
			}
		}
	}

	for _, p := range prompts {
		if other, ok := existing["dot-ai-"+p.Name]; ok && other.Source != "" && other.Source != source {
			fmt.Fprintf(os.Stderr, "warning: skipping %q: already provided by source %q (first-source-wins)\n", p.Name, RedactURL(other.Source))
			continue
		}
		rendered := renderPrompt(cfg, p.Name, repo)
		if err := writePromptSkill(outDir, p, rendered, source); err != nil {
			return "", "", &client.RequestError{
				Message:  fmt.Sprintf("Error: failed to write skill for prompt %q: %v", p.Name, err),
				ExitCode: client.ExitToolError,
			}
		}
	}

	if err := writeRoutingSkill(outDir, routingSkill); err != nil {
		return "", "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to write routing skill: %v", err),
			ExitCode: client.ExitToolError,
		}
	}

	return outDir, source, nil
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

func fetchPrompts(cfg *config.Config, repo string) ([]promptDef, string, error) {
	var params []client.Param
	if repo != "" {
		params = append(params, client.Param{Name: "repo", Value: repo, Location: "query"})
	}
	body, err := client.Do(cfg, "GET", "/api/v1/prompts", params)
	if err != nil {
		return nil, "", err
	}
	var resp promptsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to parse prompts response: %v", err),
			ExitCode: client.ExitToolError,
		}
	}
	return resp.Data.Prompts, resp.Data.Source, nil
}

// renderPrompt attempts to fetch the rendered content of a prompt.
// Returns nil if the render fails (e.g., required arguments missing).
// When repo is non-empty, it is passed through as ?repo=<url>.
func renderPrompt(cfg *config.Config, name, repo string) *promptRenderResponse {
	var params []client.Param
	if repo != "" {
		params = append(params, client.Param{Name: "repo", Value: repo, Location: "query"})
	}
	body, err := client.Do(cfg, "POST", "/api/v1/prompts/"+url.PathEscape(name), params)
	if err != nil {
		return nil
	}
	var resp promptRenderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	return &resp
}

// scanExistingSkills walks the output directory and returns a map of skill
// directory name → existingSkill (path + source-from-frontmatter). Only
// directories named dot-ai-* are returned; the routing skill ("dot-ai") and
// the lock file are excluded.
func scanExistingSkills(dir string) (map[string]existingSkill, error) {
	out := map[string]existingSkill{}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "dot-ai" || !strings.HasPrefix(name, "dot-ai-") {
			continue
		}
		skillDir := filepath.Join(dir, name)
		out[name] = existingSkill{
			Path:   skillDir,
			Source: readSkillSource(filepath.Join(skillDir, "SKILL.md")),
		}
	}
	return out, nil
}

func writeToolSkill(dir string, t toolDef, source string) error {
	skillDir := filepath.Join(dir, "dot-ai-"+t.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: dot-ai-%s\n", t.Name))
	b.WriteString(fmt.Sprintf("description: %s\n", yamlEscape(t.Description)))
	b.WriteString("user-invocable: true\n")
	if source != "" {
		b.WriteString(fmt.Sprintf("source: %s\n", yamlEscape(source)))
	}
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
		if openapi.IsPositionalCandidate(p.Required, p.Type, p.Enum) {
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

func writePromptSkill(dir string, p promptDef, rendered *promptRenderResponse, source string) error {
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
	if source != "" {
		b.WriteString(fmt.Sprintf("source: %s\n", yamlEscape(source)))
	}
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

	// Phase 1: Validate and decode all supporting files before writing
	// anything to disk. This prevents partial artifacts if a file has
	// an invalid path or bad base64.
	type decodedFile struct {
		path string
		data []byte
	}
	var decoded []decodedFile
	if rendered != nil {
		for _, f := range rendered.Data.Files {
			cleanPath := filepath.Clean(f.Path)
			if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) {
				return fmt.Errorf("invalid file path %q: path traversal not allowed", f.Path)
			}
			data, err := base64.StdEncoding.DecodeString(f.Content)
			if err != nil {
				return fmt.Errorf("decoding file %s: %w", f.Path, err)
			}
			decoded = append(decoded, decodedFile{path: cleanPath, data: data})
		}
	}

	// Phase 2: Rewrite relative file references in SKILL.md to full paths.
	// Authors write relative paths (e.g., "bash analyze.sh") and the
	// generator rewrites them based on --agent flag and dot-ai- prefix.
	content := b.String()
	if len(decoded) > 0 {
		var pairs []string
		for _, f := range decoded {
			fullPath := filepath.Join(skillDir, f.path)
			pairs = append(pairs, "./"+f.path, fullPath)
			pairs = append(pairs, f.path, fullPath)
		}
		content = strings.NewReplacer(pairs...).Replace(content)
	}

	// Phase 3: Write all files to disk.
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return err
	}
	for _, f := range decoded {
		target := filepath.Join(skillDir, f.path)
		if dir := filepath.Dir(target); dir != skillDir {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := os.WriteFile(target, f.data, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func writeRoutingSkill(dir string, content []byte) error {
	skillDir := filepath.Join(dir, "dot-ai")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0o644)
}

// RedactURL strips userinfo (user:password@) from a URL string so that any
// embedded credentials are not echoed to stdout, logs, or CI output. Returns
// the input unchanged for non-URL values (e.g. "built-in") or parse failures.
func RedactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return s
	}
	u.User = nil
	return u.String()
}

// yamlEscape wraps a string in quotes if it contains YAML-special characters.
func yamlEscape(s string) string {
	if strings.ContainsAny(s, ":#{}[]|>&*!%@`\"'\n") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
