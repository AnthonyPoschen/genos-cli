// Package cli is the genos customer command line.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/config"
	"github.com/AnthonyPoschen/genos-cli/internal/creds"
)

const usage = `genos is the customer command line for Genos game servers.

Usage:
  genos servers
  genos status <serverID>
  genos start <serverID> [--yes]
  genos stop <serverID> [--yes]
  genos force-stop <serverID> [--yes]
  genos restart <serverID> [--yes]
  genos console <serverID> <text...>
  genos setups <serverID>
  genos select-setup <serverID> <setupID> [--expected <id>]
  genos unload-setup <serverID> [--expected <id>]
  genos config get <serverID>
  genos config put <serverID> [--file path]
  genos schema <gameID>
  genos management-schema <gameID>
  genos auth login
  genos auth token
  genos auth status

The API origin is GENOS_HOST, otherwise currentHost in
$XDG_CONFIG_HOME/genos/config.toml (default ~/.config/genos/config.toml).
http is allowed only for localhost, 127.0.0.1, and genos.localhost.

The token is GENOS_TOKEN, otherwise the OS keyring item service "genos"
for that origin, otherwise credentials.json mode 0600. GENOS_TOKEN is
never written to disk. ~/.config/genos/local.env is not read.

--yes skips a required confirmation. force-stop still tells the API that
unsaved progress may be lost. Without a TTY, a required prompt exits
without sending the action.

select-setup and unload-setup send expectedSelectedSetupID for compare-
and-swap. When --expected is omitted, genos GETs setups first and uses
selectedSetupID (empty string if none). When --expected is passed, its
value is sent as-is (--expected requires a following value).

config put reads a JSON object from --file or stdin. Preferred path: the
body already includes expectedSetupID, expectedUpdatedAt, version, and
values (optional secrets). If values is present but any of those three
concurrency fields is omitted, genos GETs configuration first and fills
the missing fields (setupID, updatedAt, version). A body that is not a
JSON object or lacks values fails before any write. API errors such as
server_not_confirmed_stopped are surfaced as-is.
`

// Options configures process dependencies. Zero values use the real process.
type Options struct {
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Getenv     func(string) string
	Keyring    func() creds.Keyring
	IsTerminal func(io.Reader) bool
	OpenURL    func(string)
	Sleep      func(time.Duration)
}

// Run executes args (without argv0) and returns an exit code.
func Run(args []string, opts Options) int {
	runner := &runner{
		opts: opts,
		in:   opts.Stdin,
		out:  opts.Stdout,
		err:  opts.Stderr,
	}
	if runner.in == nil {
		runner.in = os.Stdin
	}
	if runner.out == nil {
		runner.out = os.Stdout
	}
	if runner.err == nil {
		runner.err = os.Stderr
	}
	if len(args) == 0 {
		fmt.Fprint(runner.err, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(runner.out, usage)
		return 0
	case "servers":
		return runner.servers(args[1:])
	case "status":
		return runner.status(args[1:])
	case "start", "stop", "force-stop", "restart":
		return runner.action(args[0], args[1:])
	case "console":
		return runner.console(args[1:])
	case "setups":
		return runner.setups(args[1:])
	case "select-setup":
		return runner.selectSetup(args[1:])
	case "unload-setup":
		return runner.unloadSetup(args[1:])
	case "config":
		return runner.config(args[1:])
	case "schema", "management-schema":
		return runner.schema(args[1:])
	case "auth":
		return runner.auth(args[1:])
	default:
		fmt.Fprintf(runner.err, "genos: unknown command %q\n%s", args[0], usage)
		return 2
	}
}

type runner struct {
	opts Options
	in   io.Reader
	out  io.Writer
	err  io.Writer
}

func (r *runner) getenv(key string) string {
	if r.opts.Getenv != nil {
		return r.opts.Getenv(key)
	}
	return os.Getenv(key)
}

func (r *runner) keyring() creds.Keyring {
	if r.opts.Keyring != nil {
		return r.opts.Keyring()
	}
	return creds.SystemKeyring{}
}

func (r *runner) configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	if strings.TrimSpace(r.getenv("XDG_CONFIG_HOME")) == "" && home == "" {
		return "", errors.New("cannot locate the genos config directory")
	}
	return config.Dir(r.getenv("XDG_CONFIG_HOME"), home), nil
}

func (r *runner) origin() (origin string, dir string, err error) {
	dir, err = r.configDir()
	if err != nil {
		return "", "", err
	}
	origin, err = config.ResolveOrigin(r.getenv("GENOS_HOST"), filepath.Join(dir, "config.toml"))
	if err != nil {
		return "", "", err
	}
	return origin, dir, nil
}

func (r *runner) fail(err error) int {
	fmt.Fprintf(r.err, "genos: %s\n", err)
	return 1
}

func (r *runner) usage(message string) int {
	if message != "" {
		fmt.Fprintf(r.err, "genos: %s\n", message)
	}
	fmt.Fprint(r.err, usage)
	return 2
}

func (r *runner) terminal() bool {
	if r.opts.IsTerminal != nil {
		return r.opts.IsTerminal(r.in)
	}
	return stdinIsTerminal(r.in)
}

func stdinIsTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (r *runner) confirmPrompt(prompt string) bool {
	if !r.terminal() {
		fmt.Fprintf(r.err, "genos: %s re-run with --yes\n", prompt)
		return false
	}
	fmt.Fprintf(r.err, "%s [y/N] ", prompt)
	if readYes(r.in) {
		return true
	}
	fmt.Fprintln(r.err, "not confirmed")
	return false
}

func readYes(reader io.Reader) bool {
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func splitYes(args []string) (rest []string, yes bool, err error) {
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--yes":
			yes = true
		case "-h", "--help":
			return nil, false, errHelp
		case "--":
			return append(rest, args[index+1:]...), yes, nil
		default:
			if strings.HasPrefix(args[index], "-") {
				return nil, false, fmt.Errorf("unknown flag %s", args[index])
			}
			rest = append(rest, args[index])
		}
	}
	return rest, yes, nil
}

var errHelp = errors.New("help")

func openBrowser(rawURL string) {
	command := exec.Command("xdg-open", rawURL)
	if err := command.Start(); err != nil {
		return
	}
	go func() { _ = command.Wait() }()
}

func credentialPath(dir string) string {
	return filepath.Join(dir, "credentials.json")
}
