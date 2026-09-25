package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
	"github.com/AnthonyPoschen/genos-cli/internal/confirm"
	"github.com/AnthonyPoschen/genos-cli/internal/creds"
)

func (r *runner) servers(args []string) int {
	if len(args) != 0 {
		return r.usage("servers takes no arguments")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	servers, err := client.ListServers(ctx)
	if err != nil {
		return r.fail(err)
	}
	if len(servers) == 0 {
		WriteEmpty(r.out, "No servers.")
		return 0
	}
	rows := make([][]string, 0, len(servers))
	for _, server := range servers {
		rows = append(rows, []string{server.ID, server.Name, server.Game.Name, server.Status})
	}
	WriteTable(r.out, []string{"ID", "NAME", "GAME", "STATUS"}, rows)
	return 0
}

func (r *runner) status(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("status requires a server id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	server, err := client.GetServer(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	WriteTable(r.out, []string{"ID", "NAME", "GAME", "STATUS"}, [][]string{{
		server.ID, server.Name, server.Game.Name, server.Status,
	}})
	return 0
}

func (r *runner) action(action string, args []string) int {
	rest, yes, err := splitYes(args)
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage(action + " requires a server id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	server, err := client.GetServer(ctx, rest[0])
	if err != nil {
		return r.fail(err)
	}
	decision, err := confirm.Confirm(action, confirm.Server{
		Name:           server.Name,
		PlayerCount:    playerCount(server),
		NotableUpdates: server.NotableUpdates,
	}, yes)
	if err != nil {
		return r.fail(err)
	}
	if !decision.Send && !r.confirmPrompt(decision.Prompt) {
		return 1
	}
	if err := client.RequestAction(ctx, rest[0], action, decision.ConfirmUnsavedProgressLoss); err != nil {
		return r.fail(err)
	}
	label := server.Name
	if label == "" {
		label = rest[0]
	}
	fmt.Fprintf(r.out, "requested %s for %s\n", action, label)
	return 0
}

func (r *runner) console(args []string) int {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") {
		return r.usage("console requires a server id and text")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	text := strings.Join(args[1:], " ")
	if strings.TrimSpace(text) == "" {
		return r.fail(errors.New("console text is empty"))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	response, err := client.Console(ctx, args[0], text)
	if errors.Is(err, apiclient.ErrNoInteractive) {
		return r.fail(fmt.Errorf("no interactive console channel for %s", args[0]))
	}
	if err != nil {
		return r.fail(err)
	}
	fmt.Fprintln(r.out, response)
	return 0
}

func (r *runner) api() (*apiclient.Client, error) {
	origin, dir, err := r.origin()
	if err != nil {
		return nil, err
	}
	resolved, err := creds.ResolveToken(r.env.Token, r.keyring(), creds.File{
		Path:   credentialPath(dir),
		Origin: origin,
	})
	if err != nil {
		if errors.Is(err, creds.ErrNotFound) {
			return nil, fmt.Errorf("no token for %s", origin)
		}
		return nil, err
	}
	return apiclient.New(origin, resolved.Token), nil
}

func playerCount(server apiclient.Server) int64 {
	if server.Metrics == nil || server.Metrics.PlayerCount == nil || *server.Metrics.PlayerCount <= 0 {
		return 0
	}
	return *server.Metrics.PlayerCount
}

func validateID(id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/?#\\ \t\r\n") {
		return fmt.Errorf("invalid server id %q", id)
	}
	return nil
}

func oneLine(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.ReplaceAll(value, "\n", " ")
}
