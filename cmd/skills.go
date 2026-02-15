package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
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
in the agent's skills directory. Re-running deletes existing dot-ai-* skills
and regenerates them.`,
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
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		outDir, err := skills.Generate(GetConfig(), skillsAgent, skillsPath, RoutingSkill)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Skills generated successfully in %s\n", outDir)
		if skillsInstallHook {
			if err := skills.InstallSessionHook(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "SessionStart hook installed in %s\n", ".claude/settings.json")
		}
		return nil
	},
}

func init() {
	skillsGenerateCmd.Flags().StringVar(&skillsAgent, "agent", "", "Target agent: "+strings.Join(agentNames(), ", "))
	skillsGenerateCmd.Flags().StringVar(&skillsPath, "path", "", "Override output directory (for unsupported agents)")
	skillsGenerateCmd.Flags().BoolVar(&skillsInstallHook, "install-hook", false, "Install a Claude Code SessionStart hook to regenerate skills on startup (requires --agent claude-code)")
	skillsGenerateCmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return agentNames(), cobra.ShellCompDirectiveNoFileComp
	})

	skillsCmd.AddCommand(skillsGenerateCmd)
	rootCmd.AddCommand(skillsCmd)
}
