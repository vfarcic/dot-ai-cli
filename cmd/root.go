package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/config"
)

var cfg config.Config

var rootCmd = &cobra.Command{
	Use:   "dot-ai",
	Short: "CLI for the DevOps AI Toolkit",
	Long:  "Auto-generated CLI for the DevOps AI Toolkit REST API.\nTalk to your Kubernetes clusters using AI-powered tools.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute(openapiSpec []byte) {
	RegisterDynamicCommands(openapiSpec)

	if err := rootCmd.Execute(); err != nil {
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
}

func initConfig() {
	cfg.Resolve()
}
