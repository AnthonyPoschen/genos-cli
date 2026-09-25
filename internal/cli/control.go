package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (r *runner) rename(args []string) int {
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

func (r *runner) broadcast(args []string) int {
	rest, flags, err := splitControlFlags(args, map[string]bool{"--message": true}, nil)
	if errors.Is(err, errHelp) {
		return r.usage("")
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
		return r.usage("")
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
