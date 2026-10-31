package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// readCommands are the leaf commands (path below "genos") that only read and
// take --json. Commands that already print JSON accept it as a no-op so an
// agent can pass --json to any read without knowing which ones print tables.
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
	Path        string        `json:"path"`
	Use         string        `json:"use"`
	Short       string        `json:"short"`
	ReadOnly    bool          `json:"read_only"`
	JSON        bool          `json:"json"`
	RequiresYes bool          `json:"requires_yes"`
	Flags       []commandFlag `json:"flags"`
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
	collect := func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		switch f.Name {
		case "json":
			info.JSON = true
		case "yes":
			info.RequiresYes = true
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
