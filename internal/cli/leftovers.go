package cli

import (
	"context"
	"errors"
	"strings"
	"time"
)

func (r *runner) publicRCON(args []string) int {
	rest, _, switches, err := splitControlFlagsWithSwitches(args, nil, map[string]bool{
		"--rotate": true, "--yes": true,
	})
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("public-rcon requires a server id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	rotate := switches["--rotate"]
	if rotate && !switches["--yes"] {
		return r.usage("public-rcon --rotate requires --yes (destructive credential change)")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	payload, err := client.RevealPublicRCON(ctx, rest[0], rotate)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(payload)
}

func (r *runner) serverOrder(args []string) int {
	if len(args) == 0 {
		return r.usage("server-order requires at least one server id (positional order is the new fleet order)")
	}
	for _, id := range args {
		if strings.HasPrefix(id, "-") {
			return r.usage("server-order takes server ids only (positional order is the new fleet order)")
		}
		if err := validateID(id); err != nil {
			return r.fail(err)
		}
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ordered, err := client.ReplaceServerOrder(ctx, args)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(map[string]any{"serverIDs": ordered})
}

func (r *runner) plan(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("plan requires a server id")
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
	plan, err := client.GetServerPlan(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(plan)
}
