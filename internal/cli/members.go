package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
	"github.com/spf13/cobra"
)

func ownerLabel(server apiclient.Server) string {
	if server.Access.Role == "member" && strings.TrimSpace(server.Access.OwnerEmail) != "" {
		return server.Access.OwnerEmail
	}
	return "you"
}

func (r *runner) withAPI(run func(context.Context, *apiclient.Client) error) int {
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := run(ctx, client); err != nil {
		return r.fail(err)
	}
	return 0
}

func newServerMemberCmd(r *runner) *cobra.Command {
	cmd := &cobra.Command{Use: "member", Short: "Invite or remove someone on a server you own"}
	cmd.AddCommand(newServerMemberListCmd(r), newServerMemberAddCmd(r), newServerMemberRemoveCmd(r), newServerMemberProfilesCmd(r))
	return cmd
}

func newServerMemberListCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "list <serverID>",
		Short: "List who has access to a server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.withAPI(func(ctx context.Context, client *apiclient.Client) error {
				members, err := client.ListServerMembers(ctx, args[0])
				if err != nil {
					return err
				}
				if len(members) == 0 {
					WriteEmpty(r.out, "No one else has access.")
					return nil
				}
				rows := make([][]string, 0, len(members))
				for _, member := range members {
					rows = append(rows, []string{member.Email, member.Status, member.Permission})
				}
				WriteTable(r.out, []string{"EMAIL", "STATUS", "ACCESS"}, rows)
				return nil
			}))
		},
	}
}

func newServerMemberAddCmd(r *runner) *cobra.Command {
	var profiles bool
	cmd := &cobra.Command{
		Use:   "add <serverID> <email>",
		Short: "Invite an existing Genos account to a server you own",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.withAPI(func(ctx context.Context, client *apiclient.Client) error {
				member, err := client.AddServerMember(ctx, args[0], args[1], profiles)
				if err != nil {
					return err
				}
				fmt.Fprintf(r.out, "Invited %s (%s).\n", member.Email, member.Permission)
				return nil
			}))
		},
	}
	cmd.Flags().BoolVar(&profiles, "profiles", false, "Also let them edit, add, and delete your Profiles")
	return cmd
}

func newServerMemberRemoveCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <serverID> <email>",
		Short: "Cancel an invite or remove access",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.withAPI(func(ctx context.Context, client *apiclient.Client) error {
				member, err := memberByEmail(ctx, client, args[0], args[1])
				if err != nil {
					return err
				}
				if err := client.RemoveServerMember(ctx, args[0], member.ID); err != nil {
					return err
				}
				fmt.Fprintf(r.out, "Removed %s.\n", member.Email)
				return nil
			}))
		},
	}
}

func newServerMemberProfilesCmd(r *runner) *cobra.Command {
	var on, off bool
	cmd := &cobra.Command{
		Use:   "profiles <serverID> <email>",
		Short: "Turn profile management on or off",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if on == off {
				return codeErr(r.usage("profiles requires --on or --off"))
			}
			return codeErr(r.withAPI(func(ctx context.Context, client *apiclient.Client) error {
				member, err := memberByEmail(ctx, client, args[0], args[1])
				if err != nil {
					return err
				}
				if err := client.SetServerMemberProfiles(ctx, args[0], member.ID, on); err != nil {
					return err
				}
				fmt.Fprintf(r.out, "Updated %s.\n", member.Email)
				return nil
			}))
		},
	}
	cmd.Flags().BoolVar(&on, "on", false, "Let them edit, add, and delete your Profiles")
	cmd.Flags().BoolVar(&off, "off", false, "Limit them to server controls")
	return cmd
}

func newServerInviteCmd(r *runner) *cobra.Command {
	cmd := &cobra.Command{Use: "invite", Short: "Accept or decline a server invitation"}
	cmd.AddCommand(newServerInviteListCmd(r), newServerInviteAcceptCmd(r), newServerInviteDeclineCmd(r))
	return cmd
}

func newServerInviteListCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List server invitations waiting for you",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.withAPI(func(ctx context.Context, client *apiclient.Client) error {
				invites, err := client.ListServerInvites(ctx)
				if err != nil {
					return err
				}
				if len(invites) == 0 {
					WriteEmpty(r.out, "No invitations.")
					return nil
				}
				rows := make([][]string, 0, len(invites))
				for _, invite := range invites {
					rows = append(rows, []string{invite.ID, invite.ServerName, invite.OwnerEmail, invite.Permission})
				}
				WriteTable(r.out, []string{"ID", "SERVER", "FROM", "ACCESS"}, rows)
				return nil
			}))
		},
	}
}

func newServerInviteAcceptCmd(r *runner) *cobra.Command {
	return inviteAnswerCmd(r, "accept", "Accept a server invitation", true)
}

func newServerInviteDeclineCmd(r *runner) *cobra.Command {
	return inviteAnswerCmd(r, "decline", "Decline a server invitation", false)
}

func inviteAnswerCmd(r *runner, use, short string, accept bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <inviteID>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.withAPI(func(ctx context.Context, client *apiclient.Client) error {
				if err := client.AnswerServerInvite(ctx, args[0], accept); err != nil {
					return err
				}
				if accept {
					fmt.Fprintln(r.out, "Accepted.")
				} else {
					fmt.Fprintln(r.out, "Declined.")
				}
				return nil
			}))
		},
	}
}

func newServerLeaveCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "leave <serverID>",
		Short: "Stop operating a server someone shared with you",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.withAPI(func(ctx context.Context, client *apiclient.Client) error {
				if err := client.LeaveServer(ctx, args[0]); err != nil {
					return err
				}
				fmt.Fprintln(r.out, "Left the server.")
				return nil
			}))
		},
	}
}

func memberByEmail(ctx context.Context, client *apiclient.Client, serverID string, email string) (apiclient.ServerMember, error) {
	members, err := client.ListServerMembers(ctx, serverID)
	if err != nil {
		return apiclient.ServerMember{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(email))
	for _, member := range members {
		if strings.ToLower(strings.TrimSpace(member.Email)) == needle && (member.Status == "pending" || member.Status == "active") {
			return member, nil
		}
	}
	return apiclient.ServerMember{}, fmt.Errorf("no pending invite or access for %s", email)
}
