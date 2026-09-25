package cli

import (
	"github.com/spf13/cobra"
)

func newProfileCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "profile",
		Short: "Edit library profiles",
	})
	cmd.AddCommand(
		newProfileListCmd(r),
		newProfileGamesCmd(r),
		newProfileCreateCmd(r),
		newProfileRenameCmd(r),
		newProfileDeleteCmd(r),
		newProfileConfigCmd(r),
		newProfileModsCmd(r),
		newProfileSavesCmd(r),
		newProfileCopyCmd(r),
	)
	return cmd
}

func newProfileListCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List library profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.profilesList(nil))
		},
	}
}

func newProfileGamesCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "games",
		Short: "List creatable games for library profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.profilesGames(nil))
		},
	}
}

func newProfileCreateCmd(r *runner) *cobra.Command {
	var game string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a library profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := appendFlag(nil, "--game", game, flagChanged(cmd, "game"))
			return codeErr(r.profilesCreate(rebuilt))
		},
	}
	cmd.Flags().StringVar(&game, "game", "", "Game id (required)")
	return cmd
}

func newProfileRenameCmd(r *runner) *cobra.Command {
	var expectedName string
	cmd := &cobra.Command{
		Use:   "rename <profileID> <name>",
		Short: "Rename a library profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected-name", expectedName, flagChanged(cmd, "expected-name"))
			return codeErr(r.profilesRename(rebuilt))
		},
	}
	cmd.Flags().StringVar(&expectedName, "expected-name", "", "CAS expectedName (default: autofill)")
	return cmd
}

func newProfileDeleteCmd(r *runner) *cobra.Command {
	var yes bool
	var expectedName string
	cmd := &cobra.Command{
		Use:   "delete <profileID>",
		Short: "Delete a library profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected-name", expectedName, flagChanged(cmd, "expected-name"))
			rebuilt = appendSwitch(rebuilt, "--yes", yes)
			return codeErr(r.profilesDelete(rebuilt))
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm destructive delete (required)")
	cmd.Flags().StringVar(&expectedName, "expected-name", "", "CAS expectedName (default: autofill)")
	return cmd
}

func newProfileConfigCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "config",
		Short: "Read or write library profile configuration",
	})
	cmd.AddCommand(newProfileConfigGetCmd(r), newProfileConfigPutCmd(r))
	return cmd
}

func newProfileConfigGetCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "get <profileID>",
		Short: "Get profile configuration JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.profilesConfigGet(args))
		},
	}
}

func newProfileConfigPutCmd(r *runner) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "put <profileID>",
		Short: "Put profile configuration JSON from --file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--file", file, file != "")
			return codeErr(r.profilesConfigPut(rebuilt))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "JSON file path (default: stdin)")
	return cmd
}

func newProfileCopyCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "copy",
		Short: "Copy or transfer a profile between servers",
	})
	cmd.AddCommand(
		newProfileCopyDestinationsCmd(r),
		newProfileCopyStartCmd(r),
		newProfileCopyStatusCmd(r),
	)
	return cmd
}

func newProfileCopyDestinationsCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "destinations <serverID>",
		Short: "List copy/transfer destinations for a server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.setupCopyDestinations(args))
		},
	}
}

func newProfileCopyStartCmd(r *runner) *cobra.Command {
	var destination, mode, interval, timeout string
	var wait bool
	cmd := &cobra.Command{
		Use:   "start <serverID> <profileID>",
		Short: "Start a profile copy or transfer",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--destination", destination, flagChanged(cmd, "destination"))
			rebuilt = appendFlag(rebuilt, "--mode", mode, flagChanged(cmd, "mode"))
			rebuilt = appendFlag(rebuilt, "--interval", interval, flagChanged(cmd, "interval"))
			rebuilt = appendFlag(rebuilt, "--timeout", timeout, flagChanged(cmd, "timeout"))
			rebuilt = appendSwitch(rebuilt, "--wait", wait)
			return codeErr(r.setupCopyStart(rebuilt))
		},
	}
	cmd.Flags().StringVar(&destination, "destination", "", "Destination server id (required)")
	cmd.Flags().StringVar(&mode, "mode", "", "copy or transfer (required)")
	cmd.Flags().StringVar(&interval, "interval", "", "Poll interval")
	cmd.Flags().StringVar(&timeout, "timeout", "", "Poll timeout")
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until terminal status")
	return cmd
}

func newProfileCopyStatusCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "status <serverID> <copyID>",
		Short: "Show profile copy/transfer status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.setupCopyStatus(args))
		},
	}
}
