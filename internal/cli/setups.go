package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

func (r *runner) setups(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("setups requires a server id")
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
	chooser, err := client.ListSetups(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	for _, setup := range chooser.Setups {
		mark := ""
		if setup.ID == chooser.SelectedSetupID && setup.ID != "" {
			mark = "*"
		}
		fmt.Fprintf(r.out, "%s\t%s\t%s\t%s\n", oneLine(setup.ID), oneLine(setup.Name), oneLine(setup.Game.Name), mark)
	}
	for _, game := range chooser.CreatableGames {
		fmt.Fprintf(r.out, "creatable\t%s\t%s\n", oneLine(game.ID), oneLine(game.Name))
	}
	return 0
}

func (r *runner) selectSetup(args []string) int {
	rest, expected, hasExpected, err := splitExpected(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 2 {
		return r.usage("select-setup requires a server id and setup id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(rest[1]); err != nil {
		return r.fail(fmt.Errorf("invalid setup id %q", rest[1]))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expectedID, err := resolveExpected(ctx, client, rest[0], expected, hasExpected)
	if err != nil {
		return r.fail(err)
	}
	if err := client.SelectSetup(ctx, rest[0], rest[1], expectedID); err != nil {
		return r.fail(err)
	}
	fmt.Fprintf(r.out, "selected %s for %s\n", rest[1], rest[0])
	return 0
}

func (r *runner) unloadSetup(args []string) int {
	rest, expected, hasExpected, err := splitExpected(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("unload-setup requires a server id")
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
	expectedID, err := resolveExpected(ctx, client, rest[0], expected, hasExpected)
	if err != nil {
		return r.fail(err)
	}
	if err := client.UnloadSetup(ctx, rest[0], expectedID); err != nil {
		return r.fail(err)
	}
	fmt.Fprintf(r.out, "unloaded setup for %s\n", rest[0])
	return 0
}

func resolveExpected(ctx context.Context, client *apiclient.Client, serverID, expected string, hasExpected bool) (string, error) {
	if hasExpected {
		return expected, nil
	}
	chooser, err := client.ListSetups(ctx, serverID)
	if err != nil {
		return "", err
	}
	return chooser.SelectedSetupID, nil
}

// splitExpected parses [--expected <id>] from args. --expected requires a
// following value (empty string is allowed when explicitly provided).
func splitExpected(args []string) (rest []string, expected string, hasExpected bool, err error) {
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--expected":
			if hasExpected {
				return nil, "", false, errors.New("--expected specified more than once")
			}
			if index+1 >= len(args) {
				return nil, "", false, errors.New("--expected requires a value")
			}
			index++
			expected = args[index]
			hasExpected = true
		case "-h", "--help":
			return nil, "", false, errHelp
		case "--":
			return append(rest, args[index+1:]...), expected, hasExpected, nil
		default:
			if strings.HasPrefix(args[index], "-") {
				return nil, "", false, fmt.Errorf("unknown flag %s", args[index])
			}
			rest = append(rest, args[index])
		}
	}
	return rest, expected, hasExpected, nil
}
