package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/auth"
	"github.com/vfarcic/dot-ai-cli/internal/skills"
)

func agentNames() []string {
	names := make([]string, 0, len(skills.AgentDirs))
	for k := range skills.AgentDirs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

var skillsAgent string
var skillsPath string
var skillsInstallHook bool
var skillsPullLatest bool
var skillsInclude string
var skillsExclude string
var skillsCustomOnly bool
var skillsRepo string
var skillsRepoPath string
var skillsRepoBranch string
var skillsRepoFetch string
var skillsRepoDir string
var skillsSourceLabel string

// gitTokenEnvVar is the CLI host env var whose value is forwarded as the
// X-Dot-AI-Git-Token header on prompts-override requests when --repo is in use.
const gitTokenEnvVar = "DOT_AI_GIT_TOKEN"

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage agent skills",
	Long:  "Generate and manage skills for AI coding agents (Claude Code, Cursor, Windsurf).",
}

var skillsGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate agent skills from server prompts and tools",
	Long: `Fetches prompts and tools from the dot-ai server and generates SKILL.md
files for the target agent. Each tool becomes a skill wrapping its CLI command.
Each prompt becomes a skill containing the prompt instructions.

Generated skills use a dot-ai- name prefix (e.g., dot-ai-query) and are placed
in the agent's skills directory. Each generated file is tagged with a
'source:' frontmatter recording which repo it came from. Re-running scopes its
wipe-and-replace to that source: skills from other sources are left untouched,
and cross-source name collisions are skipped with a warning (first-source-wins).
Compose multiple sources by running the command multiple times — typically as
one agent hook per source.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if skillsAgent == "" && skillsPath == "" {
			return fmt.Errorf("at least one of --agent or --path is required")
		}
		if skillsAgent != "" && skillsPath == "" {
			if _, ok := skills.AgentDirs[skillsAgent]; !ok {
				return fmt.Errorf("invalid value %q for flag --agent: must be one of [%s]", skillsAgent, strings.Join(agentNames(), ", "))
			}
		}
		if skillsInstallHook {
			if skillsAgent != "claude-code" {
				return fmt.Errorf("--install-hook requires --agent claude-code")
			}
			if skillsPath != "" {
				return fmt.Errorf("--install-hook cannot be used with --path")
			}
			// BuildHookCommand does not yet emit the --repo-dir/--repo-fetch source
			// flags (PRD #13 M5 adds the round-trip), so an installed hook would
			// regenerate WITHOUT the source — a silently broken, source-less hook.
			// Refuse the combination until M5 makes it faithful.
			if skillsRepoDir != "" || skillsRepoFetch != "" {
				return fmt.Errorf("--install-hook is not yet supported with --repo-dir/--repo-fetch (PRD #13 M5): the installed hook would regenerate without the source flag")
			}
		}
		// At most one source flag may be supplied per invocation; --repo,
		// --repo-fetch, and --repo-dir each describe a complete, distinct
		// source, so combining them is ambiguous. Name the conflicting flags.
		var sourceFlags []string
		if skillsRepo != "" {
			sourceFlags = append(sourceFlags, "--repo")
		}
		if skillsRepoFetch != "" {
			sourceFlags = append(sourceFlags, "--repo-fetch")
		}
		if skillsRepoDir != "" {
			sourceFlags = append(sourceFlags, "--repo-dir")
		}
		if len(sourceFlags) > 1 {
			return fmt.Errorf("%s are mutually exclusive; specify only one source", strings.Join(sourceFlags, ", "))
		}
		// --repo-dir and --source-label are companions: a local directory is not
		// a stable cross-machine identifier, so it needs an explicit label, and a
		// label is meaningless without a directory to apply it to.
		if skillsRepoDir != "" && skillsSourceLabel == "" {
			return fmt.Errorf("--repo-dir requires --source-label")
		}
		if skillsSourceLabel != "" && skillsRepoDir == "" {
			return fmt.Errorf("--source-label requires --repo-dir")
		}
		// --source-label becomes a server-stored identifier and feeds the
		// local:<user>-<label> prefix, so constrain its charset up front with a
		// clear usage error (defense-in-depth; the downstream sinks already escape).
		if skillsSourceLabel != "" && !skills.ValidSourceLabel(skillsSourceLabel) {
			return fmt.Errorf("--source-label %q is invalid: use only letters, digits, '.', '_', or '-' (no spaces, slashes, or control characters)", skillsSourceLabel)
		}
		// --pull-latest forces the server to pull its configured git repo, which is
		// meaningless for a CLI-uploaded source (both --repo-dir and --repo-fetch
		// upload their own content via ?source=, so there is no server-side repo to
		// refresh). Leave --pull-latest with --repo or no source flag unchanged.
		if skillsPullLatest && skillsRepoDir != "" {
			return fmt.Errorf("--pull-latest cannot be used with --repo-dir: it forces a server-side git pull, which does not apply to an uploaded local source")
		}
		if skillsPullLatest && skillsRepoFetch != "" {
			return fmt.Errorf("--pull-latest cannot be used with --repo-fetch: it forces a server-side git pull, which does not apply to a CLI-uploaded source")
		}
		// --repo-path / --repo-branch only qualify a repo-bearing source
		// (--repo or --repo-fetch). They are meaningless without one, and a
		// local --repo-dir takes no subdir/branch qualifier, so reject either
		// of those combinations as a usage error.
		if (skillsRepoPath != "" || skillsRepoBranch != "") && skillsRepo == "" && skillsRepoFetch == "" {
			return fmt.Errorf("--repo-path and --repo-branch require --repo or --repo-fetch")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ov := buildOverride()
		// --repo-dir (M2): read the local directory and upload it to the
		// ingestion endpoint, then drive list+render through ?source=<identifier>.
		// The identifier is used identically for the upload, the source:
		// frontmatter tag, and the ?source= param.
		if skillsRepoDir != "" {
			resolved, err := skills.AuthorizeRepoDir(skillsRepoDir)
			if err != nil {
				return err
			}
			identifier, err := skills.SourceIdentifier(skillsSourceLabel)
			if err != nil {
				return err
			}
			status, err := skills.UploadLocalSource(GetConfig(), resolved, identifier)
			if err != nil {
				return err
			}
			if status != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Uploaded local source as %s (%s)\n", identifier, status)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Uploaded local source as %s\n", identifier)
			}
			ov.Source = identifier
		}
		// --repo-fetch (M3): clone the repo with the HOST git stack into a temp
		// dir, then feed that clone into the same upload/list/render chain as
		// --repo-dir. The identifier is the credential-scrubbed URL (RedactURL),
		// used identically for the upload source field, the source: frontmatter
		// tag, and the ?source= param — credentials never reach the server,
		// frontmatter, logs, or stdout/stderr. --repo-branch/--repo-path qualify
		// the CLONE here (the server only ever renders the uploaded result), so
		// ov.Branch/ov.Path are consumed by the clone, not sent as query params.
		if skillsRepoFetch != "" {
			identifier := skills.RedactURL(skillsRepoFetch)
			sourceDir, cleanup, err := skills.CloneRepoFetch(skillsRepoFetch, ov.Branch, ov.Path)
			if err != nil {
				return err
			}
			defer cleanup()
			status, err := skills.UploadLocalSource(GetConfig(), sourceDir, identifier)
			if err != nil {
				return err
			}
			if status != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Uploaded local source as %s (%s)\n", identifier, status)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Uploaded local source as %s\n", identifier)
			}
			ov.Source = identifier
		}
		if skillsPullLatest {
			loaded, err := skills.RefreshPrompts(GetConfig(), ov)
			if err != nil {
				return err
			}
			if loaded > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Server skills cache refreshed (%d prompts loaded)\n", loaded)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Server skills cache refreshed")
			}
		}
		include, exclude, customOnly, err := resolveSkillFilters(cmd)
		if err != nil {
			return err
		}
		outDir, source, err := skills.Generate(GetConfig(), skillsAgent, skillsPath, include, exclude, customOnly, RoutingSkill, ov)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Skills generated successfully in %s\n", outDir)
		// Only emit Source when the user explicitly passed --repo; the no-flag
		// path must remain byte-for-byte identical to the pre-PRD-12 output
		// (Success Criteria #1).
		if cmd.Flags().Changed("repo") && source != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\n", skills.RedactURL(source))
		}
		if skillsInstallHook {
			hookCmd := skills.BuildHookCommand(include, exclude, customOnly, ov)
			if err := skills.InstallSessionHook(hookCmd); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "SessionStart hook installed in %s\n", ".claude/settings.json")
		}
		return nil
	},
}

// buildOverride assembles the prompts-repo override from the resolved flags.
// The credential is read from DOT_AI_GIT_TOKEN only when --repo is in use, so
// it is never carried (let alone forwarded) on a non-override run.
func buildOverride() skills.Override {
	ov := skills.Override{
		Repo:   skillsRepo,
		Path:   strings.TrimSpace(skillsRepoPath),
		Branch: strings.TrimSpace(skillsRepoBranch),
	}
	if skillsRepo != "" {
		ov.Token = os.Getenv(gitTokenEnvVar)
	}
	return ov
}

// resolveSkillFilters applies the standard 4-tier precedence for skill filters:
// flag > env > settings.json > default (empty).
func resolveSkillFilters(cmd *cobra.Command) (include, exclude string, customOnly bool, err error) {
	settings, err := auth.LoadSettings()
	if err != nil {
		return "", "", false, err
	}

	if cmd.Flags().Changed("include") {
		include = skillsInclude
	} else if v, ok := os.LookupEnv("DOT_AI_SKILLS_INCLUDE"); ok {
		include = v
	} else {
		include = settings.SkillsInclude
	}

	if cmd.Flags().Changed("exclude") {
		exclude = skillsExclude
	} else if v, ok := os.LookupEnv("DOT_AI_SKILLS_EXCLUDE"); ok {
		exclude = v
	} else {
		exclude = settings.SkillsExclude
	}

	if cmd.Flags().Changed("custom-only") {
		customOnly = skillsCustomOnly
	} else if v, ok := os.LookupEnv("DOT_AI_SKILLS_CUSTOM_ONLY"); ok {
		customOnly = v == "true"
	} else {
		customOnly = settings.SkillsCustomOnly == "true"
	}

	return include, exclude, customOnly, nil
}

func init() {
	skillsGenerateCmd.Flags().StringVar(&skillsAgent, "agent", "", "Target agent: "+strings.Join(agentNames(), ", "))
	skillsGenerateCmd.Flags().StringVar(&skillsPath, "path", "", "Override output directory (for unsupported agents)")
	skillsGenerateCmd.Flags().BoolVar(&skillsInstallHook, "install-hook", false, "Install a Claude Code SessionStart hook to regenerate skills on startup (requires --agent claude-code)")
	skillsGenerateCmd.Flags().BoolVar(&skillsPullLatest, "pull-latest", false, "Force the server to pull the latest skills from the git repository before generating")
	skillsGenerateCmd.Flags().StringVar(&skillsInclude, "include", "", "Regex to filter skills to include (env: DOT_AI_SKILLS_INCLUDE)")
	skillsGenerateCmd.Flags().StringVar(&skillsExclude, "exclude", "", "Regex to filter skills to exclude (env: DOT_AI_SKILLS_EXCLUDE)")
	skillsGenerateCmd.Flags().BoolVar(&skillsCustomOnly, "custom-only", false, "Only generate custom prompt skills, skip MCP tool skills (env: DOT_AI_SKILLS_CUSTOM_ONLY)")
	skillsGenerateCmd.Flags().StringVar(&skillsRepo, "repo", "", "Override the server's configured prompts repo for this invocation (passed through as ?repo=<url>). Default: server's env-var repo. When --repo points at a private cross-realm source, set DOT_AI_GIT_TOKEN in the environment to authenticate the clone; it is forwarded as the X-Dot-AI-Git-Token header on override requests only and is never logged or written to skills.")
	skillsGenerateCmd.Flags().StringVar(&skillsRepoPath, "repo-path", "", "Subdirectory within --repo to read skills from (passed through as ?path=<subdir>). Requires --repo or --repo-fetch. Default: repo root")
	skillsGenerateCmd.Flags().StringVar(&skillsRepoBranch, "repo-branch", "", "Branch of --repo to read skills from (passed through as ?branch=<branch>). Requires --repo or --repo-fetch. Default: main")
	skillsGenerateCmd.Flags().StringVar(&skillsRepoFetch, "repo-fetch", "", "Clone this git repo from the CLI host (using the host's local git stack: SSH agent, git credential helper, ~/.gitconfig) and upload it as the skill source for this invocation — for sources the server cannot reach (e.g. SSO/device-attested VPNs). Accepts optional --repo-path/--repo-branch. The source: frontmatter records the URL with any credentials scrubbed. Mutually exclusive with --repo and --repo-dir.")
	skillsGenerateCmd.Flags().StringVar(&skillsRepoDir, "repo-dir", "", "Read skills from a local directory and upload them as the skill source for this invocation (no network, no clone), then render via ?source=. Requires --source-label. Opt-in: set DOT_AI_ALLOW_REPO_DIR=1 (paths under /tmp or world-writable dirs are refused; an optional base-path allowlist is set via DOT_AI_REPO_DIR_ALLOW). Mutually exclusive with --repo and --repo-fetch.")
	skillsGenerateCmd.Flags().StringVar(&skillsSourceLabel, "source-label", "", "Stable identifier for the --repo-dir source. Auto-prefixed with the host identity for per-server uniqueness: 'source: local:<user>-<label>' (falls back to local:<host>-<label>). Required with --repo-dir.")
	skillsGenerateCmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return agentNames(), cobra.ShellCompDirectiveNoFileComp
	})

	skillsCmd.AddCommand(skillsGenerateCmd)
	rootCmd.AddCommand(skillsCmd)
}
