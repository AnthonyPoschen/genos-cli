package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

// modsTarget selects server vs library-profile mod routes.
type modsTarget struct {
	kind   string // apiclient owner kind: "servers" or "profiles"
	noun   string // "server" or "profile" for usage text
	prefix string // "mods" or "profiles mods"
}

var (
	serverModsTarget  = modsTarget{kind: "servers", noun: "server", prefix: "mods"}
	profileModsTarget = modsTarget{kind: "profiles", noun: "profile", prefix: "profiles mods"}
)

func (r *runner) mods(args []string) int {
	return r.modsDispatch(serverModsTarget, args)
}

func (r *runner) profilesMods(args []string) int {
	return r.modsDispatch(profileModsTarget, args)
}

func (r *runner) modsDispatch(target modsTarget, args []string) int {
	if len(args) == 0 {
		return r.usage(target.prefix + " requires a subcommand")
	}
	switch args[0] {
	case "list":
		return r.modsList(target, args[1:])
	case "search":
		return r.modsSearch(target, args[1:])
	case "show":
		return r.modsShow(target, args[1:])
	case "credentials":
		return r.modsCredentials(target, args[1:])
	case "credentials-clear":
		return r.modsCredentialsClear(target, args[1:])
	case "stage":
		return r.modsStage(target, args[1:])
	case "unstage":
		return r.modsUnstage(target, args[1:])
	case "discard":
		return r.modsDiscard(target, args[1:])
	case "apply":
		return r.modsApply(target, args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(r.out, usage)
		return 0
	default:
		return r.usage(fmt.Sprintf("unknown %s subcommand %q", target.prefix, args[0]))
	}
}

func (r *runner) modsList(target modsTarget, args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage(target.prefix + " list requires a " + target.noun + " id")
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
	state, err := getModsFor(ctx, client, target, args[0])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsSearch(target modsTarget, args []string) int {
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
		return r.usage(target.prefix + " search requires a " + target.noun + " id")
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
	catalog, err := searchModsFor(ctx, client, target, rest[0], query)
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(catalog)
}

func (r *runner) modsShow(target modsTarget, args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return r.usage(target.prefix + " show requires a " + target.noun + " id and provider mod id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	if strings.TrimSpace(args[1]) == "" {
		return r.usage(target.prefix + " show requires a provider mod id")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	detail, err := inspectModsFor(ctx, client, target, args[0], args[1])
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(detail)
}

func (r *runner) modsCredentials(target modsTarget, args []string) int {
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
		return r.usage(target.prefix + " credentials requires a " + target.noun + " id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	username, okUser := flags["--username"]
	token, okToken := flags["--token"]
	if !okUser || strings.TrimSpace(username) == "" || !okToken || strings.TrimSpace(token) == "" {
		return r.usage(target.prefix + " credentials requires --username and --token")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expected, err := resolveExpectedSetup(ctx, client, target, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := setModCredentialsFor(ctx, client, target, rest[0], expected, username, token)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsCredentialsClear(target modsTarget, args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{"--expected-setup": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage(target.prefix + " credentials-clear requires a " + target.noun + " id")
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
	expected, err := resolveExpectedSetup(ctx, client, target, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := clearModCredentialsFor(ctx, client, target, rest[0], expected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsStage(target modsTarget, args []string) int {
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
		return r.usage(target.prefix + " stage requires a " + target.noun + " id and provider mod id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	if strings.TrimSpace(rest[1]) == "" {
		return r.usage(target.prefix + " stage requires a provider mod id")
	}
	provider, ok := flags["--provider"]
	if !ok || strings.TrimSpace(provider) == "" {
		return r.usage(target.prefix + " stage requires --provider (e.g. factorio-mod-portal or steam-workshop)")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expected, err := resolveExpectedSetup(ctx, client, target, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := stageModFor(ctx, client, target, rest[0], expected, provider, rest[1])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsUnstage(target modsTarget, args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{"--expected-setup": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage(target.prefix + " unstage requires a " + target.noun + " id")
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
	expected, err := resolveExpectedSetup(ctx, client, target, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := unstageModFor(ctx, client, target, rest[0], expected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsDiscard(target modsTarget, args []string) int {
	rest, flags, err := splitModsFlags(args, map[string]bool{"--expected-setup": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage(target.prefix + " discard requires a " + target.noun + " id")
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
	expected, err := resolveExpectedSetup(ctx, client, target, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	state, err := discardModFor(ctx, client, target, rest[0], expected)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func (r *runner) modsApply(target modsTarget, args []string) int {
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
		return r.usage(target.prefix + " apply requires a " + target.noun + " id")
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
		state, err := getModsFor(ctx, client, target, rest[0])
		if err != nil {
			return r.fail(err)
		}
		if !hasExpected {
			expected = state.SetupID
			if strings.TrimSpace(expected) == "" && target.kind == "profiles" {
				expected = rest[0]
			}
		}
		if !hasStage {
			if state.Staged == nil || strings.TrimSpace(state.Staged.StageID) == "" {
				return r.fail(errors.New("no staged mod to apply; stage a mod first or pass --stage-id"))
			}
			stageID = state.Staged.StageID
		}
	}
	if strings.TrimSpace(expected) == "" {
		if target.kind == "profiles" {
			return r.fail(errors.New("no setupID on mods state; pass --expected-setup (profile id)"))
		}
		return r.fail(errors.New("no setupID on mods state; select a setup first or pass --expected-setup"))
	}
	if strings.TrimSpace(stageID) == "" {
		return r.fail(errors.New("no staged mod to apply; stage a mod first or pass --stage-id"))
	}
	state, err := applyModFor(ctx, client, target, rest[0], expected, stageID)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(state)
}

func resolveExpectedSetup(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID string, flags map[string]string) (string, error) {
	if expected, ok := flags["--expected-setup"]; ok {
		return expected, nil
	}
	state, err := getModsFor(ctx, client, target, ownerID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(state.SetupID) != "" {
		return state.SetupID, nil
	}
	// On library profiles the setup id is the profile itself.
	if target.kind == "profiles" {
		return ownerID, nil
	}
	return "", errors.New("no setupID on mods state; select a setup first or pass --expected-setup")
}

func getModsFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID string) (apiclient.SetupModState, error) {
	if target.kind == "profiles" {
		return client.GetProfileMods(ctx, ownerID)
	}
	return client.GetMods(ctx, ownerID)
}

func searchModsFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID string, query apiclient.ModCatalogQuery) (json.RawMessage, error) {
	if target.kind == "profiles" {
		return client.SearchProfileModCatalog(ctx, ownerID, query)
	}
	return client.SearchModCatalog(ctx, ownerID, query)
}

func inspectModsFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID, providerModID string) (json.RawMessage, error) {
	if target.kind == "profiles" {
		return client.InspectProfileModCatalog(ctx, ownerID, providerModID)
	}
	return client.InspectModCatalog(ctx, ownerID, providerModID)
}

func setModCredentialsFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID, expected, username, token string) (apiclient.SetupModState, error) {
	if target.kind == "profiles" {
		return client.SetProfileModCredentials(ctx, ownerID, expected, username, token)
	}
	return client.SetModCredentials(ctx, ownerID, expected, username, token)
}

func clearModCredentialsFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID, expected string) (apiclient.SetupModState, error) {
	if target.kind == "profiles" {
		return client.ClearProfileModCredentials(ctx, ownerID, expected)
	}
	return client.ClearModCredentials(ctx, ownerID, expected)
}

func stageModFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID, expected, provider, providerModID string) (apiclient.SetupModState, error) {
	if target.kind == "profiles" {
		return client.StageProfileMod(ctx, ownerID, expected, provider, providerModID)
	}
	return client.StageMod(ctx, ownerID, expected, provider, providerModID)
}

func unstageModFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID, expected string) (apiclient.SetupModState, error) {
	if target.kind == "profiles" {
		return client.UnstageProfileMod(ctx, ownerID, expected)
	}
	return client.UnstageMod(ctx, ownerID, expected)
}

func discardModFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID, expected string) (apiclient.SetupModState, error) {
	if target.kind == "profiles" {
		return client.DiscardProfileMod(ctx, ownerID, expected)
	}
	return client.DiscardMod(ctx, ownerID, expected)
}

func applyModFor(ctx context.Context, client *apiclient.Client, target modsTarget, ownerID, expected, stageID string) (apiclient.SetupModState, error) {
	if target.kind == "profiles" {
		return client.ApplyProfileMod(ctx, ownerID, expected, stageID)
	}
	return client.ApplyMod(ctx, ownerID, expected, stageID)
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

