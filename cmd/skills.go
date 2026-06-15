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
		}
		// --repo-path / --repo-branch only qualify a repo override; they are
		// meaningless (and silently ignored server-side) without --repo, so
		// reject that combination as a usage error.
		if (skillsRepoPath != "" || skillsRepoBranch != "") && skillsRepo == "" {
			return fmt.Errorf("--repo-path and --repo-branch require --repo")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ov := buildOverride()
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
	skillsGenerateCmd.Flags().StringVar(&skillsRepoPath, "repo-path", "", "Subdirectory within --repo to read skills from (passed through as ?path=<subdir>). Requires --repo. Default: repo root")
	skillsGenerateCmd.Flags().StringVar(&skillsRepoBranch, "repo-branch", "", "Branch of --repo to read skills from (passed through as ?branch=<branch>). Requires --repo. Default: main")
	skillsGenerateCmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return agentNames(), cobra.ShellCompDirectiveNoFileComp
	})

	skillsCmd.AddCommand(skillsGenerateCmd)
	rootCmd.AddCommand(skillsCmd)
}
