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
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Arguments   []argDef `json:"arguments"`
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

// gitTokenHeader is the request header that carries the per-request git
// credential for a prompts-repo override. Its value is a secret and is never
// logged, printed, or written to generated skill frontmatter.
const gitTokenHeader = "X-Dot-AI-Git-Token"

// Override carries the per-invocation prompts source override (PRD #15/#16/#13).
// It is one of two shapes, never both at once:
//
//   - Repo override (#15/#16): Repo is the override repo URL the *server* clones;
//     Path and Branch optionally qualify it; Token is the CLI host's
//     DOT_AI_GIT_TOKEN, forwarded as the gitTokenHeader on override requests
//     only. Emitted as ?repo=&path=&branch= on the list/render calls.
//   - Source override (#13 --repo-dir / --repo-fetch): Source is the stable
//     identifier (e.g. local:<user>-<label>) of a source the *CLI* already
//     uploaded to the ingestion endpoint. The server renders it from its cache —
//     no clone. Emitted as ?source=<identifier> on the list/render calls.
//
// All fields are optional; an Override is "active" (sends override params/header)
// when either Repo or Source is set.
type Override struct {
	Repo   string
	Path   string
	Branch string
	Token  string
	// Source is the ingested-source identifier for the --repo-dir/--repo-fetch
	// path. When set, the list/render calls carry ?source=<identifier> instead
	// of ?repo= (the user-approved render signal — option b), so the server
	// serves the CLI-uploaded source and never attempts a clone. Mutually
	// exclusive with Repo at the CLI layer.
	Source string
}

// active reports whether this override should attach query params (and, for a
// repo override, the credential header). Path, Branch, and Token only qualify a
// repo override — without a Repo they are inert (mirrors the server contract).
func (o Override) active() bool { return o.Repo != "" || o.Source != "" }

// identifier returns the human-facing source identifier for error/warning
// messages: the credential-scrubbed repo URL for a repo override, or the
// ingested-source identifier (a local:<...> / git-URL string) for a source
// override. Empty when the override is inert.
func (o Override) identifier() string {
	if o.Repo != "" {
		return RedactURL(o.Repo)
	}
	return o.Source
}

// queryParams returns the source-selection query params for the prompts list
// and render endpoints. A source override produces exactly ?source=<identifier>
// (never repo/path/branch). A repo override produces ?repo= plus optional
// path/branch — exactly the #15/#16 wire format. The two are mutually exclusive,
// so repo= and source= are never sent together.
func (o Override) queryParams() []client.Param {
	if !o.active() {
		return nil
	}
	if o.Source != "" {
		return []client.Param{{Name: "source", Value: o.Source, Location: "query"}}
	}
	params := []client.Param{{Name: "repo", Value: o.Repo, Location: "query"}}
	if o.Path != "" {
		params = append(params, client.Param{Name: "path", Value: o.Path, Location: "query"})
	}
	if o.Branch != "" {
		params = append(params, client.Param{Name: "branch", Value: o.Branch, Location: "query"})
	}
	return params
}

// bodyParams returns repo/path/branch as JSON body fields for the refresh
// endpoint (the contract places the override in the body there, not the query).
// Refresh is a repo-clone operation, so a source override (--repo-dir) sends no
// body override — the contract has no server-side pull/refresh for an ingested
// source (its content arrives via upload, not a server clone).
func (o Override) bodyParams() []client.Param {
	if o.Repo == "" {
		return nil
	}
	params := []client.Param{{Name: "repo", Value: o.Repo, Location: "body", ForceString: true}}
	if o.Path != "" {
		params = append(params, client.Param{Name: "path", Value: o.Path, Location: "body", ForceString: true})
	}
	if o.Branch != "" {
		params = append(params, client.Param{Name: "branch", Value: o.Branch, Location: "body", ForceString: true})
	}
	return params
}

// headers returns the credential header, forwarded only when the override is
// active and a token is set. A no-override run (or a run with no token) sends
// no header — the header is inert server-side without a repo, so the CLI never
// sends it on non-override requests.
func (o Override) headers() map[string]string {
	if !o.active() || o.Token == "" {
		return nil
	}
	return map[string]string{gitTokenHeader: o.Token}
}

// sourceError reframes a request-scoped 4xx from an override request as an
// actionable, per-source CLI error. The repo URL is redacted so an embedded
// credential never reaches output, and the token is never part of the message.
// Non-override errors and non-4xx errors pass through unchanged. A 401 is an
// API-level auth failure against the dot-ai server itself (not a rejection of
// the git source), so it also passes through unchanged rather than being
// mislabeled as a source rejection.
func sourceError(err error, o Override) error {
	if !o.active() {
		return err
	}
	re, ok := err.(*client.RequestError)
	if !ok || re.Status < 400 || re.Status >= 500 || re.Status == 401 {
		return err
	}
	msg := re.ServerMessage
	if msg == "" {
		msg = re.Message
	}
	return &client.RequestError{
		Message:  fmt.Sprintf("Error: skills source %s rejected: %s", o.identifier(), client.RedactCredentials(msg)),
		ExitCode: re.ExitCode,
		Status:   re.Status,
	}
}

// isEvictedSourceError reports whether err is the specific 400 the server
// returns when a ?source= override names an ingested source it no longer holds
// (the in-memory LRU evicted it or a restart cleared it). It is the trigger for
// the single force-re-upload + retry in Generate, so it is deliberately narrow:
//
//   - only for an active SOURCE override (ov.Source != "") — a --repo override
//     never uploads, so its 400s must keep flowing to the clean error path;
//   - status 400 exactly (the server's VALIDATION_ERROR for an unknown source);
//   - the message both mentions "upload" AND points at the ingestion endpoint
//     (POST /api/v1/prompts/sources) — the server's re-upload guidance, not any
//     other 400 (an over-limit upload, a bad path, etc.).
//
// Matching the guidance text (not just the status) keeps an unrelated 400 from
// triggering a pointless re-upload loop. err is the already-reframed error from
// fetchPrompts (sourceError preserves Status and folds the server message into
// Message), so both substrings are still present to match.
func isEvictedSourceError(err error, ov Override) bool {
	if ov.Source == "" {
		return false
	}
	re, ok := err.(*client.RequestError)
	if !ok || re.Status != 400 {
		return false
	}
	// err is always the reframed error from fetchPrompts → sourceError, which
	// folds the server message into Message and leaves ServerMessage empty, so
	// Message is the only field that ever carries the guidance text to match.
	msg := re.Message
	return strings.Contains(strings.ToLower(msg), "upload") &&
		strings.Contains(msg, "/api/v1/prompts/sources")
}

// RefreshPrompts asks the server to pull the latest prompts from the
// configured git repository, busting the server-side cache. When the override
// is active, repo/path/branch are sent as JSON body fields and the credential
// as the gitTokenHeader (the refresh contract is body-based, not query-based).
// It returns the number of prompts the server reports loading (0 if the server
// did not report a count); a parse failure on an otherwise-successful refresh
// is non-fatal and yields a count of 0.
func RefreshPrompts(cfg *config.Config, ov Override) (int, error) {
	body, err := client.DoWithHeaders(cfg, "POST", "/api/v1/prompts/refresh", ov.bodyParams(), ov.headers())
	if err != nil {
		return 0, sourceError(err, ov)
	}
	var resp struct {
		Data struct {
			PromptsLoaded int `json:"promptsLoaded"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Data.PromptsLoaded, nil
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
// embedded routing skill content to write as dot-ai/SKILL.md. The ov override
// (PRD #15 + #16) is threaded onto every prompts request the run makes: when
// active, repo/path/branch are sent as ?repo=&path=&branch= query params on the
// prompts list and render calls, and the credential is forwarded as the
// X-Dot-AI-Git-Token header — overriding the server's configured default repo.
//
// Per PRD #12, Generate performs a per-source wipe-and-replace: only skills
// whose `source:` frontmatter matches the current invocation's source are
// removed before the fresh set is written. Skills from other sources are left
// untouched. Cross-source name collisions are resolved first-source-wins with
// a warning to stderr. An exclusive file lock on <outDir>/.dot-ai.lock
// serializes concurrent invocations.
func Generate(cfg *config.Config, agent, path, include, exclude string, customOnly bool, routingSkill []byte, ov Override, ensureUploaded func(force bool) error) (string, string, error) {
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

	// For a CLI-uploaded source (--repo-dir/--repo-fetch) ensureUploaded gates
	// the upload on the source's content hash: the first ever run uploads, an
	// unchanged source skips, a changed source re-uploads. It MUST run before the
	// list/render calls below, which resolve the source from the server's cache
	// via ?source=. Nil for --repo / the no-flag path (no upload to gate).
	if ensureUploaded != nil {
		if err := ensureUploaded(false); err != nil {
			return "", "", err
		}
	}

	var tools []toolDef
	if !customOnly {
		tools, err = fetchTools(cfg)
		if err != nil {
			return "", "", err
		}
	}

	prompts, source, err := fetchPrompts(cfg, ov)
	// Evict-retry: the server's ingested-source cache is in-memory/LRU and does
	// not survive a restart, so a gated run may skip the upload only for the
	// list ?source= to find the source gone (a 400 with re-upload guidance). When
	// that exact condition is detected, force a single re-upload and retry the
	// list ONCE; if it still 400s, fall through to the clean error below (no
	// loop). Applies to both --repo-dir and --repo-fetch (ensureUploaded != nil).
	if err != nil && ensureUploaded != nil && isEvictedSourceError(err, ov) {
		if upErr := ensureUploaded(true); upErr != nil {
			return "", "", upErr
		}
		prompts, source, err = fetchPrompts(cfg, ov)
	}
	if err != nil {
		return "", "", err
	}

	// For a source override (--repo-dir/--repo-fetch) the CLI owns the source
	// identity: skills must be tagged with the SAME identifier used for the
	// upload and the ?source= param, not whatever the server echoes back on the
	// list call. Using the server echo would tag with the server's default
	// source and break wipe-own-slice for this source.
	if ov.Source != "" {
		source = ov.Source
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
	wipeUntagged := !ov.active()
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
		rendered, rerr := renderPrompt(cfg, p.Name, ov)
		if rerr != nil {
			// Never fail silently. A request-scoped override 4xx is reframed by
			// sourceError into an actionable "skills source <repo> rejected: ..."
			// message (repo URL + any embedded credential redacted); every other
			// failure — a non-override per-prompt error (e.g. missing required
			// args), a transient 5xx, an API 401, a connection error, or a parse
			// failure — passes through with its own message. We warn-and-continue
			// rather than abort so a single prompt failing does not sink the run,
			// then fall back to metadata-only output below (rendered == nil).
			// RedactCredentials is applied defensively so no token or
			// credentialed URL can leak to stderr even if a message slipped past
			// the server's own scrubbing.
			warn := client.RedactCredentials(sourceError(rerr, ov).Error())
			fmt.Fprintf(os.Stderr, "warning: failed to render prompt %q: %s\n", p.Name, warn)
		}
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

func fetchPrompts(cfg *config.Config, ov Override) ([]promptDef, string, error) {
	body, err := client.DoWithHeaders(cfg, "GET", "/api/v1/prompts", ov.queryParams(), ov.headers())
	if err != nil {
		return nil, "", sourceError(err, ov)
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

// renderPrompt attempts to fetch the rendered content of a prompt. It returns
// the parsed response, or a nil response together with the underlying error so
// the caller can decide how to handle it (e.g. a request-scoped override 4xx
// must be surfaced, not silently swallowed — PRD #16). The active override is
// threaded through as ?repo=&path=&branch= query params plus the credential
// header, so each render call is scoped to the same source as the list call.
func renderPrompt(cfg *config.Config, name string, ov Override) (*promptRenderResponse, error) {
	body, err := client.DoWithHeaders(cfg, "POST", "/api/v1/prompts/"+url.PathEscape(name), ov.queryParams(), ov.headers())
	if err != nil {
		return nil, err
	}
	var resp promptRenderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to parse render response for prompt %q: %v", name, err),
			ExitCode: client.ExitToolError,
		}
	}
	return &resp, nil
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
// embedded credentials are not echoed to stdout, logs, or CI output, and it is
// the ONLY scrub on the --repo-fetch success-path identifier (upload source
// field, source: frontmatter tag, ?source= param). A parser-only redaction is
// brittle: net/url returns the input unchanged on a parse error, and may not
// surface every credential as u.User (a non-RFC-3986 URL, or odd encodings), so
// a user:token@ could otherwise survive to a sink. To make it belt-and-
// suspenders, the result is ALSO run through client.RedactCredentials (a regex
// scrub of any embedded "...@" in free text): on a clean parse it strips any
// residual credential u.User missed, and on a parse failure / userinfo-less
// parse it becomes the sole, robust line of defense. For values with no embedded
// credential (e.g. "built-in" or a plain https URL) RedactCredentials is a
// no-op, so this stays a faithful pass-through there.
func RedactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return client.RedactCredentials(s)
	}
	u.User = nil
	return client.RedactCredentials(u.String())
}

// yamlEscape wraps a string in quotes if it contains YAML-special characters.
func yamlEscape(s string) string {
	if strings.ContainsAny(s, ":#{}[]|>&*!%@`\"'\n") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
