package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

func (r *runner) setupCopy(args []string) int {
	if len(args) == 0 {
		return r.usage("setup-copy requires destinations, start, or status")
	}
	switch args[0] {
	case "destinations":
		return r.setupCopyDestinations(args[1:])
	case "start":
		return r.setupCopyStart(args[1:])
	case "status":
		return r.setupCopyStatus(args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(r.out, usage)
		return 0
	default:
		return r.usage(fmt.Sprintf("unknown setup-copy subcommand %q", args[0]))
	}
}

func (r *runner) setupCopyDestinations(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("setup-copy destinations requires a server id")
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
	destinations, err := client.ListSetupCopyDestinations(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(map[string]any{"destinations": destinations})
}

func (r *runner) setupCopyStart(args []string) int {
	rest, flags, switches, err := splitControlFlagsWithSwitches(args, map[string]bool{
		"--destination": true,
		"--mode":        true,
		"--interval":    true,
		"--timeout":     true,
	}, map[string]bool{"--wait": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 2 {
		return r.usage("setup-copy start requires a server id, setup id, --destination, and --mode")
	}
	destination := strings.TrimSpace(flags["--destination"])
	mode := strings.TrimSpace(flags["--mode"])
	if destination == "" || mode == "" {
		return r.usage("setup-copy start requires --destination <serverID> and --mode copy|transfer")
	}
	if mode != "copy" && mode != "transfer" {
		return r.usage("--mode must be copy or transfer")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(rest[1]); err != nil {
		return r.fail(fmt.Errorf("invalid setup id %q", rest[1]))
	}
	if err := validateID(destination); err != nil {
		return r.fail(fmt.Errorf("invalid destination server id %q", destination))
	}
	interval, timeout, err := resolvePollTiming(flagSetFromMaps(flags, switches), defaultSavePollInterval, defaultExportTimeout)
	if err != nil {
		return r.usage(err.Error())
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Minute)
	defer cancel()
	copyJob, err := client.StartSetupCopy(ctx, rest[0], rest[1], destination, mode)
	if err != nil {
		return r.fail(err)
	}
	if !switches["--wait"] {
		return r.printJSON(copyJob)
	}
	deadline := time.Now().Add(timeout)
	copyJob, err = client.WaitSetupCopy(ctx, rest[0], copyJob.ID, interval, deadline, r.sleep, func(current apiclient.SetupCopy) {
		fmt.Fprintf(r.err, "genos: setup-copy %s %s %d%%\n", current.ID, current.Status, current.ProgressPercent)
	})
	if err != nil {
		return r.fail(err)
	}
	if apiclient.SetupCopyFailed(copyJob.Status) {
		_ = r.printJSON(copyJob)
		return r.fail(fmt.Errorf("setup copy %s", copyJob.Status))
	}
	return r.printJSON(copyJob)
}

func (r *runner) setupCopyStatus(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return r.usage("setup-copy status requires a server id and copy id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(args[1]); err != nil {
		return r.fail(fmt.Errorf("invalid copy id %q", args[1]))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	copyJob, err := client.GetSetupCopy(ctx, args[0], args[1])
	if err != nil {
		return r.fail(err)
	}
	if apiclient.SetupCopyFailed(copyJob.Status) {
		_ = r.printJSON(copyJob)
		return r.fail(fmt.Errorf("setup copy %s", copyJob.Status))
	}
	return r.printJSON(copyJob)
}

// flagSetFromMaps adapts control-flag maps to the saves poll timing helper.
func flagSetFromMaps(values map[string]string, bools map[string]bool) savesFlagSet {
	return savesFlagSet{values: values, bools: bools}
}
