package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/auth"
)

var authNoBrowser bool

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long:  "Authenticate with the dot-ai server using OAuth or manage auth state.",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate via OAuth (opens browser)",
	Long: `Starts an OAuth Authorization Code flow with PKCE.

Opens your browser to the Dex login page. After authentication,
the token is stored in ~/.config/dot-ai/credentials.json and used
automatically for subsequent commands.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := GetConfig().ServerURL
		return auth.Login(serverURL, authNoBrowser)
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored OAuth credentials",
	Long: `Removes OAuth session tokens from credentials.json.

Static tokens (auth_token) are preserved. Only the OAuth session
fields (access_token, token_type, expires_at, client_id, client_secret)
are cleared.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := auth.Logout(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Logged out. Stored OAuth credentials removed.")
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		// Check for overrides first (flag/env take priority over stored credentials).
		if flagToken := GetConfig().Token; flagToken != "" {
			source := "flag"
			if os.Getenv("DOT_AI_AUTH_TOKEN") != "" {
				source = "env"
			}
			fmt.Fprintf(out, "Authenticated via: Static token (%s)\n", source)
			return nil
		}

		info, err := auth.Status()
		if err != nil {
			return err
		}
		switch info.Mode {
		case "oauth":
			fmt.Fprintln(out, "Authenticated via: OAuth")
			fmt.Fprintf(out, "Token: %s\n", info.Token)
			if info.ExpiresAt != "" {
				fmt.Fprintf(out, "Token expires: %s\n", info.ExpiresAt)
				if info.Expired {
					fmt.Fprintln(out, "Status: EXPIRED — run 'dot-ai auth login' to re-authenticate")
				} else {
					fmt.Fprintln(out, "Status: Valid")
				}
			}
		case "static-token":
			fmt.Fprintln(out, "Authenticated via: Static token")
			fmt.Fprintf(out, "Token: %s\n", info.Token)
		default:
			fmt.Fprintln(out, "Not authenticated.")
			fmt.Fprintln(out, "Run 'dot-ai auth login' or set --token / DOT_AI_AUTH_TOKEN.")
		}
		return nil
	},
}

func init() {
	authLoginCmd.Flags().BoolVar(&authNoBrowser, "no-browser", false, "Don't open browser; print the login URL instead")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}
