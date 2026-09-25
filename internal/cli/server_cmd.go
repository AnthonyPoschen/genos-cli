package cli

import (
	"github.com/spf13/cobra"
)

func newServerCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "server",
		Short: "Operate a game server",
	})
	cmd.AddCommand(
		newServerListCmd(r),
		newServerStatusCmd(r),
		newServerActionCmd(r, "start", "Start a game server"),
		newServerActionCmd(r, "stop", "Stop a game server"),
		newServerActionCmd(r, "force-stop", "Force-stop a game server (may lose unsaved progress)"),
		newServerActionCmd(r, "restart", "Restart a game server"),
		newServerRenameCmd(r),
		newServerOrderCmd(r),
		newServerPlanCmd(r),
		newServerConfigCmd(r),
		newServerModsCmd(r),
		newServerSavesCmd(r),
		newServerProfileCmd(r),
	)
	return cmd
}

func newServerListCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List owned game servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.servers(nil))
		},
	}
}

func newServerStatusCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "status <serverID>",
		Short: "Show one server's status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.status(args))
		},
	}
}

func newServerActionCmd(r *runner, action, short string) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   action + " <serverID>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendSwitch(rebuilt, "--yes", yes)
			return codeErr(r.action(action, rebuilt))
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation prompt")
	return cmd
}

func newServerRenameCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <serverID> <name>",
		Short: "Rename a game server",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.rename(args))
		},
	}
}

func newServerOrderCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "order <serverID> [<serverID>...]",
		Short: "Replace fleet display order (every owned server exactly once)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.serverOrder(args))
		},
	}
}

func newServerPlanCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "plan <serverID>",
		Short: "Show the server plan (read-only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.plan(args))
		},
	}
}

func newServerConfigCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "config",
		Short: "Read or write server configuration",
	})
	cmd.AddCommand(newServerConfigGetCmd(r), newServerConfigPutCmd(r))
	return cmd
}

func newServerConfigGetCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "get <serverID>",
		Short: "Get server configuration JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.configGet(args))
		},
	}
}

func newServerConfigPutCmd(r *runner) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "put <serverID>",
		Short: "Put server configuration JSON from --file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--file", file, file != "")
			return codeErr(r.configPut(rebuilt))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "JSON file path (default: stdin)")
	return cmd
}

func newServerProfileCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "profile",
		Short: "List, select, or unload the attached profile",
	})
	cmd.AddCommand(
		newServerProfileListCmd(r),
		newServerProfileSelectCmd(r),
		newServerProfileUnloadCmd(r),
	)
	return cmd
}

func newServerProfileListCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "list <serverID>",
		Short: "List profiles available on a server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.setups(args))
		},
	}
}

func newServerProfileSelectCmd(r *runner) *cobra.Command {
	var expected string
	cmd := &cobra.Command{
		Use:   "select <serverID> <profileID>",
		Short: "Attach a library profile to the server",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected", expected, flagChanged(cmd, "expected"))
			return codeErr(r.selectSetup(rebuilt))
		},
	}
	cmd.Flags().StringVar(&expected, "expected", "", "expectedSelectedSetupID for CAS (default: autofill from GET)")
	return cmd
}

func newServerProfileUnloadCmd(r *runner) *cobra.Command {
	var expected string
	cmd := &cobra.Command{
		Use:   "unload <serverID>",
		Short: "Unload the attached profile from the server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected", expected, flagChanged(cmd, "expected"))
			return codeErr(r.unloadSetup(rebuilt))
		},
	}
	cmd.Flags().StringVar(&expected, "expected", "", "expectedSelectedSetupID for CAS (default: autofill from GET)")
	return cmd
}
