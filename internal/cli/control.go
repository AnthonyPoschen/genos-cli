package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

func (r *runner) rename(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return r.usage("rename requires a server id and a name")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	name := args[1]
	if strings.TrimSpace(name) == "" {
		return r.fail(errors.New("server name must not be empty"))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	server, err := client.RenameServer(ctx, args[0], name)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(server)
}

func (r *runner) createSetup(args []string) int {
	rest, flags, err := splitControlFlags(args, map[string]bool{"--game": true}, nil)
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("create-setup requires a server id and --game <gameID>")
	}
	gameID := strings.TrimSpace(flags["--game"])
	if gameID == "" {
		return r.usage("create-setup requires --game <gameID>")
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
	result, err := client.CreateSetup(ctx, rest[0], gameID)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(result)
}

func (r *runner) renameSetup(args []string) int {
	rest, flags, switches, err := splitControlFlagsWithSwitches(args, map[string]bool{
		"--expected": true, "--expected-name": true,
	}, nil)
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	_ = switches
	if len(rest) != 3 {
		return r.usage("rename-setup requires a server id, setup id, and name")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(rest[1]); err != nil {
		return r.fail(fmt.Errorf("invalid setup id %q", rest[1]))
	}
	name := rest[2]
	if strings.TrimSpace(name) == "" {
		return r.fail(errors.New("setup name must not be empty"))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expectedName, expectedSelected, err := resolveSetupMutationExpected(ctx, client, rest[0], rest[1], flags)
	if err != nil {
		return r.fail(err)
	}
	result, err := client.RenameSetup(ctx, rest[0], rest[1], name, expectedName, expectedSelected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(result)
}

func (r *runner) deleteSetup(args []string) int {
	rest, flags, switches, err := splitControlFlagsWithSwitches(args, map[string]bool{
		"--expected": true, "--expected-name": true,
	}, map[string]bool{"--yes": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 2 {
		return r.usage("delete-setup requires a server id, setup id, and --yes")
	}
	if !switches["--yes"] {
		return r.usage("delete-setup requires --yes (destructive)")
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
	expectedName, expectedSelected, err := resolveSetupMutationExpected(ctx, client, rest[0], rest[1], flags)
	if err != nil {
		return r.fail(err)
	}
	result, err := client.DeleteSetup(ctx, rest[0], rest[1], expectedName, expectedSelected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(result)
}

func (r *runner) broadcast(args []string) int {
	rest, flags, err := splitControlFlags(args, map[string]bool{"--message": true}, nil)
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("broadcast requires a server id")
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
	payload, err := client.SendBroadcast(ctx, rest[0], flags["--message"])
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(payload)
}

func (r *runner) broadcastStatus(args []string) int {
	rest, flags, err := splitControlFlags(args, map[string]bool{"--message": true}, nil)
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("broadcast-status requires a server id")
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
	payload, err := client.GetBroadcast(ctx, rest[0], flags["--message"])
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(payload)
}

func resolveSetupMutationExpected(ctx context.Context, client *apiclient.Client, serverID, setupID string, flags map[string]string) (expectedName, expectedSelected string, err error) {
	_, hasExpectedName := flags["--expected-name"]
	_, hasExpected := flags["--expected"]
	if hasExpectedName && hasExpected {
		return flags["--expected-name"], flags["--expected"], nil
	}
	chooser, err := client.ListSetups(ctx, serverID)
	if err != nil {
		return "", "", err
	}
	expectedName = flags["--expected-name"]
	if !hasExpectedName {
		found := false
		for _, setup := range chooser.Setups {
			if setup.ID == setupID {
				expectedName = setup.Name
				found = true
				break
			}
		}
		if !found {
			return "", "", fmt.Errorf("setup %s not found on server %s; pass --expected-name", setupID, serverID)
		}
	}
	expectedSelected = flags["--expected"]
	if !hasExpected {
		expectedSelected = chooser.SelectedSetupID
	}
	return expectedName, expectedSelected, nil
}

func splitControlFlags(args []string, valued map[string]bool, switches map[string]bool) (rest []string, flags map[string]string, err error) {
	rest, flags, _, err = splitControlFlagsWithSwitches(args, valued, switches)
	return rest, flags, err
}

func splitControlFlagsWithSwitches(args []string, valued map[string]bool, switches map[string]bool) (rest []string, flags map[string]string, switchFlags map[string]bool, err error) {
	flags = map[string]string{}
	switchFlags = map[string]bool{}
	if valued == nil {
		valued = map[string]bool{}
	}
	if switches == nil {
		switches = map[string]bool{}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "-h", "--help":
			return nil, nil, nil, errHelp
		case "--":
			return append(rest, args[index+1:]...), flags, switchFlags, nil
		}
		if switches[arg] {
			if switchFlags[arg] {
				return nil, nil, nil, fmt.Errorf("%s specified more than once", arg)
			}
			switchFlags[arg] = true
			continue
		}
		if valued[arg] {
			if _, exists := flags[arg]; exists {
				return nil, nil, nil, fmt.Errorf("%s specified more than once", arg)
			}
			if index+1 >= len(args) {
				return nil, nil, nil, fmt.Errorf("%s requires a value", arg)
			}
			index++
			flags[arg] = args[index]
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return nil, nil, nil, fmt.Errorf("unknown flag %s", arg)
		}
		rest = append(rest, arg)
	}
	return rest, flags, switchFlags, nil
}
