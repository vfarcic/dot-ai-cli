package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/skills"
)

var skillsAgent string
var skillsPath string

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
				return fmt.Errorf("invalid value %q for flag --agent: must be one of [claude-code, cursor, windsurf]", skillsAgent)
			}
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		outDir, err := skills.Generate(GetConfig(), skillsAgent, skillsPath, RoutingSkill)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
			if reqErr, ok := err.(*client.RequestError); ok {
				os.Exit(reqErr.ExitCode)
			}
			os.Exit(client.ExitToolError)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Skills generated successfully in %s\n", outDir)
		return nil
	},
}

func init() {
	skillsGenerateCmd.Flags().StringVar(&skillsAgent, "agent", "", "Target agent: claude-code, cursor, windsurf")
	skillsGenerateCmd.Flags().StringVar(&skillsPath, "path", "", "Override output directory (for unsupported agents)")
	skillsGenerateCmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"claude-code", "cursor", "windsurf"}, cobra.ShellCompDirectiveNoFileComp
	})

	skillsCmd.AddCommand(skillsGenerateCmd)
	rootCmd.AddCommand(skillsCmd)
}
