package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Every runnable command (path below "genos") is in exactly one of
// readCommands or writeCommands; TestCommandClassification fails CI on an
// unclassified, double-classified or stale entry.
//
// readCommands only read and take --json. Commands that already print JSON
// accept it as a no-op so an agent can pass --json to any read without
// knowing which ones print tables.
var readCommands = []string{
	"server list",
	"server status",
	"server plan",
	"server config get",
	"server profile list",
	"server mods list",
	"server mods search",
	"server mods show",
	"server saves export-status",
	"server saves import-status",
	"profile list",
	"profile games",
	"profile config get",
	"profile mods list",
	"profile mods search",
	"profile mods show",
	"profile saves export-status",
	"profile saves import-status",
	"profile copy destinations",
	"profile copy status",
	"rcon broadcast-status",
	"auth status",
	"auth tokens",
	"me",
	"catalog",
	"dashboard",
	"schema",
	"commands",
}

// writeCommands change state, reveal or create a secret, or store credentials.
var writeCommands = []string{
	"auth login",
	"auth token",
	"auth token-create",
	"auth token-revoke",
	"profile config put",
	"profile copy start",
	"profile create",
	"profile delete",
	"profile mods apply",
	"profile mods credentials",
	"profile mods credentials-clear",
	"profile mods discard",
	"profile mods draft",
	"profile mods draft-apply",
	"profile mods import",
	"profile mods stage",
	"profile mods unstage",
	"profile rename",
	"profile saves export",
	"profile saves import",
	"profile saves import-replace",
	"profile saves import-validate",
	"rcon broadcast",
	"rcon public-credential",
	"rcon send",
	"server config put",
	"server force-stop",
	"server mods apply",
	"server mods credentials",
	"server mods credentials-clear",
	"server mods discard",
	"server mods draft",
	"server mods draft-apply",
	"server mods import",
	"server mods stage",
	"server mods unstage",
	"server order",
	"server profile select",
	"server profile unload",
	"server rename",
	"server restart",
	"server saves export",
	"server saves import",
	"server saves import-replace",
	"server saves import-validate",
	"server start",
	"server stop"}

const readAnnotation = "genos.read"

// addReadJSONFlags marks each read command and gives it --json bound to
// r.jsonOut unless the command already defines its own --json.
func addReadJSONFlags(root *cobra.Command, r *runner) {
	for _, path := range readCommands {
		cmd, rest, err := root.Find(strings.Fields(path))
		if err != nil || len(rest) != 0 || cmd == root {
			panic("read command not found: " + path)
		}
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[readAnnotation] = "true"
		// All read commands share r.jsonOut: one process runs one command,
		// so only the invoked command's flag can set it.
		if cmd.Flags().Lookup("json") == nil {
			cmd.Flags().BoolVar(&r.jsonOut, "json", false, "Print JSON")
		}
	}
}

type commandFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default"`
	Usage     string `json:"usage"`
}

type commandInfo struct {
	Path       string        `json:"path"`
	Use        string        `json:"use"`
	Short      string        `json:"short"`
	ReadOnly   bool          `json:"read_only"`
	Mutating   bool          `json:"mutating"`
	JSON       bool          `json:"json"`
	HasYesFlag bool          `json:"has_yes_flag"`
	Flags      []commandFlag `json:"flags"`
}

func newCommandsCmd(r *runner) *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List every command (use --json for a machine-readable catalog)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			infos := commandCatalog(cmd.Root())
			if r.jsonOut {
				return codeErr(r.printJSON(map[string]any{"commands": infos}))
			}
			rows := make([][]string, 0, len(infos))
			for _, info := range infos {
				rows = append(rows, []string{info.Path, info.Short})
			}
			WriteTable(r.out, []string{"COMMAND", "DESCRIPTION"}, rows)
			return nil
		},
	}
}

// commandCatalog lists runnable leaf commands, sorted by path. Global flags
// (--no-input) are listed on every command because they apply everywhere.
func commandCatalog(root *cobra.Command) []commandInfo {
	var infos []commandInfo
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		children := cmd.Commands()
		if cmd != root && len(children) == 0 && !cmd.Hidden {
			infos = append(infos, describeCommand(cmd))
		}
		for _, child := range children {
			if child.Hidden || child.Name() == "help" {
				continue
			}
			walk(child)
		}
	}
	walk(root)
	sort.Slice(infos, func(i, j int) bool { return infos[i].Path < infos[j].Path })
	return infos
}

func describeCommand(cmd *cobra.Command) commandInfo {
	info := commandInfo{
		Path:     strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" "),
		Use:      cmd.UseLine(),
		Short:    cmd.Short,
		ReadOnly: cmd.Annotations[readAnnotation] == "true",
		Flags:    []commandFlag{},
	}
	info.Mutating = !info.ReadOnly
	collect := func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		switch f.Name {
		case "json":
			info.JSON = true
		case "yes":
			info.HasYesFlag = true
		}
		info.Flags = append(info.Flags, commandFlag{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Default:   f.DefValue,
			Usage:     f.Usage,
		})
	}
	cmd.LocalFlags().VisitAll(collect)
	cmd.InheritedFlags().VisitAll(collect)
	return info
}
