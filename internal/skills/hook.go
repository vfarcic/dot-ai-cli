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
	settingsFile      = ".claude/settings.json"
	hookCommandBase   = "dot-ai skills generate --agent claude-code"
	hookCommandPrefix = "dot-ai skills generate"
	hookMatcher       = "startup"
	hookType          = "command"
	hookEventKey      = "SessionStart"
)

// HookSource carries the raw per-invocation source-flag values an installed hook
// must re-emit verbatim so each session-start regenerates the SAME source. It is
// needed because ov.Source alone cannot reconstruct the original flags: for
// --repo-dir it is the derived local:<user>-<label> identifier (which loses both
// the directory path and the label), and for --repo-fetch it is the
// credential-scrubbed URL. The CLI flag values are passed straight through here
// (RepoFetch is scrubbed at emit time). No credential or opt-in env var is ever
// stored — see BuildHookCommand.
type HookSource struct {
	RepoFetch   string // --repo-fetch raw URL (emitted RedactURL-scrubbed)
	RepoDir     string // --repo-dir local directory path
	SourceLabel string // --source-label (required companion of --repo-dir)
	NoCache     bool   // --no-cache (only meaningful with --repo-fetch)
}

// shellQuote wraps a value in POSIX single quotes so a shell that executes the
// stored hook command treats it as a literal string. A single-quoted shell word
// undergoes NO expansion — $, backtick, $(...), and ${...} are all inert — which
// is exactly what closes the M5 shell-injection: BuildHookCommand's output is
// stored in .claude/settings.json and run BY Claude Code THROUGH A SHELL, where
// Go %q (double-quote) escaping would still let those metacharacters execute
// (e.g. a --repo-dir path /x/$(touch /tmp/PWNED) runs the substitution). The
// only character that cannot appear inside single quotes is a single quote, so
// it is escaped by the standard POSIX idiom: close the quote, emit a backslash-
// escaped quote, reopen the quote — i.e. each ' in the value becomes '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildHookCommand constructs the hook command string from the resolved flags,
// mirroring the arguments passed to `skills generate`. The emitted command is
// scoped to exactly one source so each hook firing reproduces it (PRD #12
// hook-per-source model, extended in PRD #16 and PRD #13). Every interpolated
// value is shell-quoted (see shellQuote) because the command is run through a
// shell at hook-run time:
//
//   - --repo (or no source flag): --repo/--repo-path/--repo-branch are appended
//     from the override. The --repo URL is credential-SCRUBBED (RedactURL) so a
//     credentialed URL never reaches settings.json; the token itself is NEVER
//     embedded — it is read from DOT_AI_GIT_TOKEN at hook-run time.
//   - --repo-fetch: the credential-SCRUBBED URL (RedactURL) is emitted — a
//     credentialed URL must never reach settings.json — plus --repo-path/
//     --repo-branch (which qualify the clone) and --no-cache when set.
//   - --repo-dir: the directory path and its required --source-label. The
//     DOT_AI_ALLOW_REPO_DIR opt-in is deliberately NOT embedded; settings.json
//     is often committed/shared, and baking the opt-in would let anyone who
//     clones the repo side-load from that path without consent. A --repo-dir
//     hook therefore reads DOT_AI_ALLOW_REPO_DIR from the env at hook-run time,
//     exactly like --repo reads DOT_AI_GIT_TOKEN.
//
// When no new source flag is set the output is argv-identical to the pre-PRD-13
// command string (Backward Compatibility): the quoting bytes changed from Go %q
// double quotes to POSIX single quotes, but the shell parses both to the SAME
// argument vector.
func BuildHookCommand(include, exclude string, customOnly bool, ov Override, src HookSource) string {
	cmd := hookCommandBase
	if customOnly {
		cmd += " --custom-only"
	}
	if include != "" {
		cmd += " --include " + shellQuote(include)
	}
	if exclude != "" {
		cmd += " --exclude " + shellQuote(exclude)
	}
	switch {
	case src.RepoFetch != "":
		// Scrub any embedded credential before it can reach settings.json.
		cmd += " --repo-fetch " + shellQuote(RedactURL(src.RepoFetch))
		if ov.Path != "" {
			cmd += " --repo-path " + shellQuote(ov.Path)
		}
		if ov.Branch != "" {
			cmd += " --repo-branch " + shellQuote(ov.Branch)
		}
		if src.NoCache {
			cmd += " --no-cache"
		}
	case src.RepoDir != "":
		cmd += " --repo-dir " + shellQuote(src.RepoDir)
		cmd += " --source-label " + shellQuote(src.SourceLabel)
	default:
		// --repo / no source flag. The --repo URL is scrubbed (RedactURL) so a
		// credentialed URL never lands in settings.json (the token is read from
		// DOT_AI_GIT_TOKEN at hook-run time, never embedded).
		if ov.Repo != "" {
			cmd += " --repo " + shellQuote(RedactURL(ov.Repo))
		}
		if ov.Path != "" {
			cmd += " --repo-path " + shellQuote(ov.Path)
		}
		if ov.Branch != "" {
			cmd += " --repo-branch " + shellQuote(ov.Branch)
		}
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
