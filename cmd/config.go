package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/auth"
)

// configKey maps a CLI key name to its Settings field.
type configKey struct {
	CLI         string
	Description string
	Default     string
	Get         func(*auth.Settings) string
	Set         func(*auth.Settings, string)
}

var knownKeys = []configKey{
	{
		CLI:         "server-url",
		Description: "Server URL",
		Default:     "",
		Get:         func(s *auth.Settings) string { return s.ServerURL },
		Set:         func(s *auth.Settings, v string) { s.ServerURL = v },
	},
	{
		CLI:         "output-format",
		Description: "Output format (json, yaml)",
		Default:     "yaml",
		Get:         func(s *auth.Settings) string { return s.OutputFormat },
		Set:         func(s *auth.Settings, v string) { s.OutputFormat = v },
	},
	{
		CLI:         "skills.include",
		Description: "Regex for skills to include",
		Default:     "",
		Get:         func(s *auth.Settings) string { return s.SkillsInclude },
		Set:         func(s *auth.Settings, v string) { s.SkillsInclude = v },
	},
	{
		CLI:         "skills.exclude",
		Description: "Regex for skills to exclude",
		Default:     "",
		Get:         func(s *auth.Settings) string { return s.SkillsExclude },
		Set:         func(s *auth.Settings, v string) { s.SkillsExclude = v },
	},
}

func findKey(name string) *configKey {
	for i := range knownKeys {
		if knownKeys[i].CLI == name {
			return &knownKeys[i]
		}
	}
	return nil
}

func validKeyNames() string {
	names := make([]string, len(knownKeys))
	for i, k := range knownKeys {
		names[i] = k.CLI
	}
	return strings.Join(names, ", ")
}

func unknownKeyError(name string) error {
	return fmt.Errorf("unknown key %q. Valid keys: %s", name, validKeyNames())
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage persistent settings",
	Long:  "Read and write settings in ~/.config/dot-ai/settings.json.",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: fmt.Sprintf(`Set a persistent configuration value in settings.json.

Supported keys:
  %s`, func() string {
		var lines []string
		for _, k := range knownKeys {
			lines = append(lines, fmt.Sprintf("%-16s %s", k.CLI, k.Description))
		}
		return strings.Join(lines, "\n  ")
	}()),
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := findKey(args[0])
		if key == nil {
			return unknownKeyError(args[0])
		}
		s, err := auth.LoadSettings()
		if err != nil {
			return err
		}
		key.Set(&s, args[1])
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", key.CLI, args[1])
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := findKey(args[0])
		if key == nil {
			return unknownKeyError(args[0])
		}
		s, err := auth.LoadSettings()
		if err != nil {
			return err
		}
		val := key.Get(&s)
		if val == "" {
			if key.Default != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (default)\n", key.Default)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "(not set)")
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), val)
		}
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration keys and values",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := auth.LoadSettings()
		if err != nil {
			return err
		}
		for _, key := range knownKeys {
			val := key.Get(&s)
			if val == "" {
				if key.Default != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (default)\n", key.CLI, key.Default)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: (not set)\n", key.CLI)
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", key.CLI, val)
			}
		}
		return nil
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset <key>",
	Short: "Reset a configuration value to its default",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := findKey(args[0])
		if key == nil {
			return unknownKeyError(args[0])
		}
		s, err := auth.LoadSettings()
		if err != nil {
			return err
		}
		key.Set(&s, "")
		if err := s.Save(); err != nil {
			return err
		}
		if key.Default != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: reset to default (%s)\n", key.CLI, key.Default)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: reset\n", key.CLI)
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configSetCmd, configGetCmd, configListCmd, configResetCmd)
	rootCmd.AddCommand(configCmd)
}
