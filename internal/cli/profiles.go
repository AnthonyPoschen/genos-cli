package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

func (r *runner) profiles(args []string) int {
	if len(args) == 0 {
		return r.profilesList(nil)
	}
	switch args[0] {
	case "list":
		return r.profilesList(args[1:])
	case "games":
		return r.profilesGames(args[1:])
	case "create":
		return r.profilesCreate(args[1:])
	case "rename":
		return r.profilesRename(args[1:])
	case "delete":
		return r.profilesDelete(args[1:])
	case "config":
		return r.profilesConfig(args[1:])
	case "mods":
		return r.profilesMods(args[1:])
	case "saves":
		return r.profilesSaves(args[1:])
	case "-h", "--help":
		return r.usage("")
	default:
		return r.usage(fmt.Sprintf("unknown profiles subcommand %q", args[0]))
	}
}

func (r *runner) profilesList(args []string) int {
	if len(args) != 0 {
		return r.usage("profiles list takes no arguments")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dashboard, err := client.GetDashboard(ctx)
	if err != nil {
		return r.fail(err)
	}
	if len(dashboard.Profiles) == 0 {
		WriteEmpty(r.out, "No profiles.")
	} else {
		rows := make([][]string, 0, len(dashboard.Profiles))
		for _, profile := range dashboard.Profiles {
			mark := ""
			if profile.Selected {
				mark = "*"
			}
			rows = append(rows, []string{profile.ID, profile.Name, profile.Game.Name, profile.ServerName, mark})
		}
		WriteTable(r.out, []string{"ID", "NAME", "GAME", "SERVER", "SELECTED"}, rows)
	}
	fmt.Fprintln(r.out)
	fmt.Fprintf(r.out, "Capacity: %d/%d\n", dashboard.ProfileCapacity.Used, dashboard.ProfileCapacity.Limit)
	return 0
}

func (r *runner) profilesGames(args []string) int {
	if len(args) != 0 {
		return r.usage("profiles games takes no arguments")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	options, err := client.LibraryProfileOptions(ctx)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(options)
}

func (r *runner) profilesCreate(args []string) int {
	rest, flags, err := splitControlFlags(args, map[string]bool{"--game": true}, nil)
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 0 {
		return r.usage("profiles create requires --game <gameID>")
	}
	gameID := strings.TrimSpace(flags["--game"])
	if gameID == "" {
		return r.usage("profiles create requires --game <gameID>")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := client.CreateLibraryProfile(ctx, gameID)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(result)
}

func (r *runner) profilesRename(args []string) int {
	rest, flags, err := splitControlFlags(args, map[string]bool{"--expected-name": true}, nil)
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 2 {
		return r.usage("profiles rename requires a profile id and a name")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(fmt.Errorf("invalid profile id %q", rest[0]))
	}
	name := rest[1]
	if strings.TrimSpace(name) == "" {
		return r.fail(errors.New("profile name must not be empty"))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expectedName, err := resolveProfileExpectedName(ctx, client, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	result, err := client.RenameLibraryProfile(ctx, rest[0], name, expectedName)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(result)
}

func (r *runner) profilesDelete(args []string) int {
	rest, flags, switches, err := splitControlFlagsWithSwitches(args, map[string]bool{
		"--expected-name": true,
	}, map[string]bool{"--yes": true})
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("profiles delete requires a profile id and --yes")
	}
	if !switches["--yes"] {
		return r.usage("profiles delete requires --yes (destructive)")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(fmt.Errorf("invalid profile id %q", rest[0]))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	expectedName, err := resolveProfileExpectedName(ctx, client, rest[0], flags)
	if err != nil {
		return r.fail(err)
	}
	result, err := client.DeleteLibraryProfile(ctx, rest[0], expectedName)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(result)
}

func (r *runner) profilesConfig(args []string) int {
	if len(args) == 0 {
		return r.usage("profiles config requires get or put")
	}
	switch args[0] {
	case "get":
		return r.profilesConfigGet(args[1:])
	case "put":
		return r.profilesConfigPut(args[1:])
	case "-h", "--help":
		return r.usage("")
	default:
		return r.usage(fmt.Sprintf("unknown profiles config subcommand %q", args[0]))
	}
}

func (r *runner) profilesConfigGet(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("profiles config get requires a profile id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(fmt.Errorf("invalid profile id %q", args[0]))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	configuration, err := client.GetProfileConfiguration(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(configuration)
}

func (r *runner) profilesConfigPut(args []string) int {
	rest, filePath, err := splitFile(args)
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("profiles config put requires a profile id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(fmt.Errorf("invalid profile id %q", rest[0]))
	}
	raw, err := r.readPutBody(filePath)
	if err != nil {
		return r.fail(err)
	}
	body, err := parseConfigurationPut(raw)
	if err != nil {
		return r.fail(err)
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	body, err = fillProfileConfigurationPut(ctx, client, rest[0], body)
	if err != nil {
		return r.fail(err)
	}
	configuration, err := client.PutProfileConfiguration(ctx, rest[0], body)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(configuration)
}

func resolveProfileExpectedName(ctx context.Context, client *apiclient.Client, profileID string, flags map[string]string) (string, error) {
	if expected, ok := flags["--expected-name"]; ok {
		return expected, nil
	}
	dashboard, err := client.GetDashboard(ctx)
	if err != nil {
		return "", err
	}
	for _, profile := range dashboard.Profiles {
		if profile.ID == profileID {
			return profile.Name, nil
		}
	}
	return "", fmt.Errorf("profile %s not found on dashboard; pass --expected-name", profileID)
}

func fillProfileConfigurationPut(ctx context.Context, client *apiclient.Client, profileID string, body map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	needSetup := missingRaw(body, "expectedSetupID")
	needUpdated := missingRaw(body, "expectedUpdatedAt")
	needVersion := missingRaw(body, "version")
	if !needSetup && !needUpdated && !needVersion {
		return body, nil
	}
	live, err := client.GetProfileConfiguration(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if needSetup {
		encoded, err := json.Marshal(live.SetupID)
		if err != nil {
			return nil, err
		}
		body["expectedSetupID"] = encoded
	}
	if needUpdated {
		encoded, err := json.Marshal(live.UpdatedAt)
		if err != nil {
			return nil, err
		}
		body["expectedUpdatedAt"] = encoded
	}
	if needVersion {
		encoded, err := json.Marshal(live.Version)
		if err != nil {
			return nil, err
		}
		body["version"] = encoded
	}
	return body, nil
}
