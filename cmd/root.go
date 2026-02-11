package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/config"
)

var cfg config.Config

var rootCmd = &cobra.Command{
	Use:          "dot-ai",
	Short:        "CLI for the DevOps AI Toolkit",
	Long:         "Auto-generated CLI for the DevOps AI Toolkit REST API.\nTalk to your Kubernetes clusters using AI-powered tools.",
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var RoutingSkill []byte

func Execute(openapiSpec, routingSkill []byte) {
	RoutingSkill = routingSkill
	RegisterDynamicCommands(openapiSpec)

	if err := rootCmd.Execute(); err != nil {
		var reqErr *client.RequestError
		if errors.As(err, &reqErr) {
			os.Exit(reqErr.ExitCode)
		}
		os.Exit(client.ExitUsageError)
	}
}

func GetConfig() *config.Config {
	return &cfg
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfg.ServerURL, "server-url", "", "Server URL (env: DOT_AI_URL)")
	rootCmd.PersistentFlags().StringVar(&cfg.Token, "token", "", "Authentication token (env: DOT_AI_AUTH_TOKEN)")
	rootCmd.PersistentFlags().StringVar(&cfg.OutputFormat, "output", "", "Output format: json, yaml (default: yaml) (env: DOT_AI_OUTPUT_FORMAT)")
	rootCmd.RegisterFlagCompletionFunc("output", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func initConfig() {
	cfg.Resolve()
}
