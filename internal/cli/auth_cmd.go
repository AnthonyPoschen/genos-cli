package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAuthCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "auth",
		Short: "Authenticate and manage access tokens",
	})
	cmd.AddCommand(
		newAuthLoginCmd(r),
		newAuthTokenCmd(r),
		newAuthStatusCmd(r),
		newAuthTokensCmd(r),
		newAuthTokenCreateCmd(r),
		newAuthTokenRevokeCmd(r),
	)
	return cmd
}

func newAuthLoginCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Device-code login and store the token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.login())
		},
	}
}

func newAuthTokenCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Store an access token from stdin",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				fmt.Fprintln(r.err, "genos: read the token from stdin; do not pass it as an argument")
				return codeErr(2)
			}
			return codeErr(r.storeStdin())
		},
	}
}

func newAuthStatusCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show resolved origin and token source",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.authStatus())
		},
	}
}

func newAuthTokensCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "tokens",
		Short: "List personal access token metadata",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.authTokens(nil))
		},
	}
}

func newAuthTokenCreateCmd(r *runner) *cobra.Command {
	var name, clientName, machineName string
	cmd := &cobra.Command{
		Use:   "token-create",
		Short: "Create a PAT (secret printed once on stdout)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := []string{}
			rebuilt = appendFlag(rebuilt, "--name", name, flagChanged(cmd, "name"))
			rebuilt = appendFlag(rebuilt, "--client-name", clientName, flagChanged(cmd, "client-name"))
			rebuilt = appendFlag(rebuilt, "--machine-name", machineName, flagChanged(cmd, "machine-name"))
			return codeErr(r.authTokenCreate(rebuilt))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Token name")
	cmd.Flags().StringVar(&clientName, "client-name", "", "Client name")
	cmd.Flags().StringVar(&machineName, "machine-name", "", "Machine name")
	return cmd
}

func newAuthTokenRevokeCmd(r *runner) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "token-revoke <tokenID>",
		Short: "Revoke a personal access token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendSwitch(rebuilt, "--yes", yes)
			return codeErr(r.authTokenRevoke(rebuilt))
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm revoke (required)")
	return cmd
}
