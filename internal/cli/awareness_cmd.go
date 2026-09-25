package cli

import (
	"github.com/spf13/cobra"
)

func newCatalogCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "Show the game catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.catalog(nil))
		},
	}
}

func newMeCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the authenticated account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.me(nil))
		},
	}
}

func newDashboardCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Show the account dashboard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.dashboard(nil))
		},
	}
}

func newSchemaCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "schema <gameID>",
		Short: "Show the management schema for a game",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.schema(args))
		},
	}
}
