package cli

import (
	"github.com/spf13/cobra"
)

func newRconCmd(r *runner) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "rcon",
		Short: "In-game / RCON interaction",
	})
	cmd.AddCommand(
		newRconSendCmd(r),
		newRconBroadcastCmd(r),
		newRconBroadcastStatusCmd(r),
		newRconPublicCredentialCmd(r),
	)
	return cmd
}

func newRconSendCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "send <serverID> <text...>",
		Short: "Send text on the interactive console channel",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.console(args))
		},
	}
}

func newRconBroadcastCmd(r *runner) *cobra.Command {
	var message string
	cmd := &cobra.Command{
		Use:   "broadcast <serverID>",
		Short: "Send an in-game broadcast notice",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--message", message, flagChanged(cmd, "message"))
			return codeErr(r.broadcast(rebuilt))
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "Broadcast message text")
	return cmd
}

func newRconBroadcastStatusCmd(r *runner) *cobra.Command {
	var message string
	cmd := &cobra.Command{
		Use:   "broadcast-status <serverID>",
		Short: "Show broadcast availability / preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--message", message, flagChanged(cmd, "message"))
			return codeErr(r.broadcastStatus(rebuilt))
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "Optional message for preview")
	return cmd
}

func newRconPublicCredentialCmd(r *runner) *cobra.Command {
	var rotate, yes bool
	cmd := &cobra.Command{
		Use:   "public-credential <serverID>",
		Short: "Reveal (or rotate) the public RCON password once",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendSwitch(rebuilt, "--rotate", rotate)
			rebuilt = appendSwitch(rebuilt, "--yes", yes)
			return codeErr(r.publicRCON(rebuilt))
		},
	}
	cmd.Flags().BoolVar(&rotate, "rotate", false, "Rotate the credential (requires --yes)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm credential rotation")
	return cmd
}
