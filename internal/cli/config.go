package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

func (r *runner) config(args []string) int {
	if len(args) == 0 {
		return r.usage("config requires get or put")
	}
	switch args[0] {
	case "get":
		return r.configGet(args[1:])
	case "put":
		return r.configPut(args[1:])
	case "-h", "--help":
		return r.usage("")
	default:
		return r.usage(fmt.Sprintf("unknown config subcommand %q", args[0]))
	}
}

func (r *runner) configGet(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("config get requires a server id")
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
	configuration, err := client.GetConfiguration(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(configuration)
}

func (r *runner) configPut(args []string) int {
	rest, filePath, err := splitFile(args)
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("config put requires a server id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
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
	body, err = fillConfigurationPut(ctx, client, rest[0], body)
	if err != nil {
		return r.fail(err)
	}
	configuration, err := client.PutConfiguration(ctx, rest[0], body)
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(configuration)
}

func (r *runner) schema(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return r.usage("schema requires a game id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(fmt.Errorf("invalid game id %q", args[0]))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	schema, err := client.GetManagementSchema(ctx, args[0])
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(schema)
}

func (r *runner) printJSON(value any) int {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return r.fail(err)
	}
	fmt.Fprintln(r.out, string(encoded))
	return 0
}

func (r *runner) printRawJSON(raw json.RawMessage) int {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return r.fail(err)
	}
	return r.printJSON(value)
}

func (r *runner) readPutBody(filePath string) ([]byte, error) {
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	data, err := io.ReadAll(r.in)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// parseConfigurationPut requires a JSON object with a values member.
func parseConfigurationPut(raw []byte) (map[string]json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, errors.New("config put body is empty; pass JSON on stdin or --file")
	}
	var probe any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return nil, fmt.Errorf("config put body must be a JSON object: %w", err)
	}
	object, ok := probe.(map[string]any)
	if !ok {
		return nil, errors.New("config put body must be a JSON object with values")
	}
	if _, ok := object["values"]; !ok {
		return nil, errors.New("config put body must include values")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &body); err != nil {
		return nil, fmt.Errorf("config put body must be a JSON object: %w", err)
	}
	return body, nil
}

// fillConfigurationPut copies missing concurrency fields from a live GET.
// Preferred agent path already includes expectedSetupID, expectedUpdatedAt, and version.
func fillConfigurationPut(ctx context.Context, client *apiclient.Client, serverID string, body map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	needSetup := missingRaw(body, "expectedSetupID")
	needUpdated := missingRaw(body, "expectedUpdatedAt")
	needVersion := missingRaw(body, "version")
	if !needSetup && !needUpdated && !needVersion {
		return body, nil
	}
	live, err := client.GetConfiguration(ctx, serverID)
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

func missingRaw(body map[string]json.RawMessage, key string) bool {
	_, ok := body[key]
	return !ok
}

// splitFile parses [--file path] from args.
func splitFile(args []string) (rest []string, filePath string, err error) {
	var hasFile bool
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--file":
			if hasFile {
				return nil, "", errors.New("--file specified more than once")
			}
			if index+1 >= len(args) {
				return nil, "", errors.New("--file requires a path")
			}
			index++
			filePath = args[index]
			hasFile = true
		case "-h", "--help":
			return nil, "", errHelp
		case "--":
			return append(rest, args[index+1:]...), filePath, nil
		default:
			if strings.HasPrefix(args[index], "-") {
				return nil, "", fmt.Errorf("unknown flag %s", args[index])
			}
			rest = append(rest, args[index])
		}
	}
	return rest, filePath, nil
}
