package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

func (r *runner) mods(args []string) int {
	if len(args) == 0 {
		return r.usage("mods requires a subcommand")
	}
	switch args[0] {
	case "list":
		return r.modsList(args[1:])
	case "search":
		return r.modsSearch(args[1:])
	case "show":
		return r.modsShow(args[1:])
	case "credentials":
		return r.modsCredentials(args[1:])
	case "credentials-clear":
		return r.modsCredentialsClear(args[1:])
	case "stage":
		return r.modsStage(args[1:])
	case "unstage":
		return r.modsUnstage(args[1:])
	case "discard":
		return r.modsDiscard(args[1:])
	case "apply":
		return r.modsApply(args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(r.out, usage)
		return 0
	default:
		return r.usage(fmt.Sprintf("unknown mods subcommand %q", args[0]))
	}
}

func (r *runner) modsList(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("mods list requires a server id")
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
	state, err := client.GetMods(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsSearch(args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{
		"--query": true, "--category": true, "--sort": true, "--page": true, "--page-size": true,
	})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("mods search requires a server id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	query := apiclient.ModCatalogQuery{
		Query:    flags["--query"],
		Category: flags["--category"],
		Sort:     flags["--sort"],
	}
	if raw, ok := flags["--page"]; ok {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return r.usage("--page must be a positive integer")
		}
		query.Page = page
	}
	if raw, ok := flags["--page-size"]; ok {
		size, err := strconv.Atoi(raw)
		if err != nil || size < 1 {
			return r.usage("--page-size must be a positive integer")
		}
		query.PageSize = size
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	catalog, err := client.SearchModCatalog(ctx, rest[0], query)
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(catalog)
}

func (r *runner) modsShow(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return r.usage("mods show requires a server id and provider mod id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	if strings.TrimSpace(args[1]) == "" {
		return r.usage("mods show requires a provider mod id")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	detail, err := client.InspectModCatalog(ctx, args[0], args[1])
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(detail)
}

func (r *runner) modsCredentials(args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{
		"--username": true, "--token": true, "--expected-setup": true,
	})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("mods credentials requires a server id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	username, okUser := flags["--username"]
	token, okToken := flags["--token"]
	if !okUser || strings.TrimSpace(username) == "" || !okToken || strings.TrimSpace(token) == "" {
		return r.usage("mods credentials requires --username and --token")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expected, err := resolveExpectedSetup(ctx, client, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := client.SetModCredentials(ctx, rest[0], expected, username, token)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsCredentialsClear(args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{"--expected-setup": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("mods credentials-clear requires a server id")
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
	expected, err := resolveExpectedSetup(ctx, client, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := client.ClearModCredentials(ctx, rest[0], expected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsStage(args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{
		"--provider": true, "--expected-setup": true,
	})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 2 {
		return r.usage("mods stage requires a server id and provider mod id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	if strings.TrimSpace(rest[1]) == "" {
		return r.usage("mods stage requires a provider mod id")
	}
	provider, ok := flags["--provider"]
	if !ok || strings.TrimSpace(provider) == "" {
		return r.usage("mods stage requires --provider (e.g. factorio-mod-portal or steam-workshop)")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expected, err := resolveExpectedSetup(ctx, client, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := client.StageMod(ctx, rest[0], expected, provider, rest[1])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsUnstage(args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{"--expected-setup": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("mods unstage requires a server id")
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
	expected, err := resolveExpectedSetup(ctx, client, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := client.UnstageMod(ctx, rest[0], expected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsDiscard(args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{"--expected-setup": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("mods discard requires a server id")
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
	expected, err := resolveExpectedSetup(ctx, client, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := client.DiscardMod(ctx, rest[0], expected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsApply(args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{
		"--stage-id": true, "--expected-setup": true,
	})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("mods apply requires a server id")
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

	expected, hasExpected := flags["--expected-setup"]
	stageID, hasStage := flags["--stage-id"]
	if !hasExpected || !hasStage {
		state, err := client.GetMods(ctx, rest[0])
		if err != nil {
			return r.fail(err)
		}
		if !hasExpected {
			expected = state.SetupID
		}
		if !hasStage {
			if state.Staged == nil || strings.TrimSpace(state.Staged.StageID) == "" {
				return r.fail(errors.New("no staged mod to apply; stage a mod first or pass --stage-id"))
			}
			stageID = state.Staged.StageID
		}
	}
	if strings.TrimSpace(expected) == "" {
		return r.fail(errors.New("no setupID on mods state; select a setup first or pass --expected-setup"))
	}
	if strings.TrimSpace(stageID) == "" {
		return r.fail(errors.New("no staged mod to apply; stage a mod first or pass --stage-id"))
	}
	state, err := client.ApplyMod(ctx, rest[0], expected, stageID)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func resolveExpectedSetup(ctx context.Context, client *apiclient.Client, serverID string, flags map[string]string) (string, error) {
	if expected, ok := flags["--expected-setup"]; ok {
		return expected, nil
	}
	state, err := client.GetMods(ctx, serverID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(state.SetupID) == "" {
		return "", errors.New("no setupID on mods state; select a setup first or pass --expected-setup")
	}
	return state.SetupID, nil
}

// splitModsFlags parses known --flag value pairs. Boolean presence flags are not used.
func splitModsFlags(args []string, valued map[string]bool) (rest []string, flags map[string]string, err error) {
	flags = map[string]string{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "-h", "--help":
			return nil, nil, errHelp
		case "--":
			return append(rest, args[index+1:]...), flags, nil
		}
		if valued[arg] {
			if _, exists := flags[arg]; exists {
				return nil, nil, fmt.Errorf("%s specified more than once", arg)
			}
			if index+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s requires a value", arg)
			}
			index++
			flags[arg] = args[index]
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return nil, nil, fmt.Errorf("unknown flag %s", arg)
		}
		rest = append(rest, arg)
	}
	return rest, flags, nil
}
