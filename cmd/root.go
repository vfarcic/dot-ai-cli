package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/config"
	"github.com/vfarcic/dot-ai-cli/internal/rbac"
)

var cfg config.Config

var rootCmd = &cobra.Command{
	Use:          "dot-ai",
	Short:        "CLI for the DevOps AI Toolkit",
	Long:         "Auto-generated CLI for the DevOps AI Toolkit REST API.\nTalk to your Kubernetes clusters using AI-powered tools.",
	SilenceUsage: true,
	// SilenceErrors stops cobra from printing the error itself. Several
	// RequestError.Message values already embed a leading "Error:" prefix, so
	// letting cobra prefix again produced "Error: Error: ...". With cobra
	// silenced, Execute is the single place that prints, via printError, which
	// normalizes to exactly one prefix.
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var RoutingSkill []byte

func Execute(openapiSpec, routingSkill []byte, version string) {
	rootCmd.Version = version
	RoutingSkill = routingSkill
	RegisterDynamicCommands(openapiSpec)

	if err := rootCmd.Execute(); err != nil {
		printError(err)
		var reqErr *client.RequestError
		if errors.As(err, &reqErr) {
			os.Exit(reqErr.ExitCode)
		}
		os.Exit(client.ExitUsageError)
	}
}

// printError writes err to stderr with exactly one "Error:" prefix. Because
// rootCmd has SilenceErrors set, this is the only place errors reach the user,
// so it must always print. Some RequestError.Message values already start with
// "Error:" while plain fmt.Errorf failures do not; stripping any leading
// (case-insensitive) "Error:" before re-adding one normalizes both to a single
// prefix without altering the underlying message wording.
func printError(err error) {
	msg := strings.TrimSpace(err.Error())
	if len(msg) >= 6 && strings.EqualFold(msg[:6], "Error:") {
		msg = strings.TrimSpace(msg[6:])
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
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
	if err := cfg.Resolve(); err != nil {
		printError(err)
		os.Exit(1)
	}

	if !isCompletionInvocation() {
		rbac.FilterCommands(rootCmd, &cfg)
	}
}

// isCompletionInvocation returns true when the CLI was invoked for shell
// completion, where latency from a network call would hurt responsiveness.
func isCompletionInvocation() bool {
	if len(os.Args) < 2 {
		return false
	}
	return os.Args[1] == "__complete" || os.Args[1] == "completion"
}
