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
  genos create-setup <serverID> --game <gameID>
  genos rename-setup <serverID> <setupID> <name> [--expected-name <name>] [--expected <id>]
  genos delete-setup <serverID> <setupID> --yes [--expected-name <name>] [--expected <id>]
  genos rename <serverID> <name>
  genos broadcast <serverID> [--message <text>]
  genos broadcast-status <serverID> [--message <text>]
  genos config get <serverID>
  genos config put <serverID> [--file path]
  genos schema <gameID>
  genos management-schema <gameID>
  genos mods list <serverID>
  genos mods search <serverID> [--query q] [--category c] [--sort s] [--page n] [--page-size n]
  genos mods show <serverID> <providerModID>
  genos mods credentials <serverID> --username U --token T [--expected-setup ID]
  genos mods credentials-clear <serverID> [--expected-setup ID]
  genos mods stage <serverID> <providerModID> --provider ID [--expected-setup ID]
  genos mods unstage <serverID> [--expected-setup ID]
  genos mods discard <serverID> [--expected-setup ID]
  genos mods apply <serverID> [--stage-id ID] [--expected-setup ID]
  genos mods draft <serverID> --provider ID [--mod-id ID ...] [--expected-setup ID]
  genos mods import <serverID> [--file path.json] [--expected-setup ID]
  genos mods draft-apply <serverID> [--expected-setup ID] [--expected-revision N]
  genos saves export <serverID> [--wait] [--interval 2s] [--timeout 15m] [--output path]
  genos saves export-status <serverID> <exportID>
  genos saves import <serverID> --file path.zip [--media-type application/zip] [--wait] [--interval 2s] [--timeout 30m] [--apply-save-mods|--no-apply-save-mods] [--yes]
  genos saves import-status <serverID> <importID>
  genos saves import-validate <serverID> <importID>
  genos saves import-replace <serverID> <importID> --yes [--apply-save-mods|--no-apply-save-mods]
  genos me
  genos dashboard
  genos catalog
  genos profiles
  genos profiles list
  genos profiles games
  genos profiles create --game <gameID>
  genos profiles rename <profileID> <name> [--expected-name <name>]
  genos profiles delete <profileID> --yes [--expected-name <name>]
  genos profiles config get <profileID>
  genos profiles config put <profileID> [--file path]
  genos profiles mods list <profileID>
  genos profiles mods search <profileID> [--query q] [--category c] [--sort s] [--page n] [--page-size n]
  genos profiles mods show <profileID> <providerModID>
  genos profiles mods credentials <profileID> --username U --token T [--expected-setup ID]
  genos profiles mods credentials-clear <profileID> [--expected-setup ID]
  genos profiles mods stage <profileID> <providerModID> --provider ID [--expected-setup ID]
  genos profiles mods unstage <profileID> [--expected-setup ID]
  genos profiles mods discard <profileID> [--expected-setup ID]
  genos profiles mods apply <profileID> [--stage-id ID] [--expected-setup ID]
  genos profiles mods draft <profileID> --provider ID [--mod-id ID ...] [--expected-setup ID]
  genos profiles mods import <profileID> [--file path.json] [--expected-setup ID]
  genos profiles mods draft-apply <profileID> [--expected-setup ID] [--expected-revision N]
  genos profiles saves export <profileID> [--wait] [--interval 2s] [--timeout 15m] [--output path]
  genos profiles saves export-status <profileID> <exportID>
  genos profiles saves import <profileID> --file path.zip [--media-type application/zip] [--wait] [--interval 2s] [--timeout 30m] [--apply-save-mods|--no-apply-save-mods] [--yes]
  genos profiles saves import-status <profileID> <importID>
  genos profiles saves import-validate <profileID> <importID>
  genos profiles saves import-replace <profileID> <importID> --yes [--apply-save-mods|--no-apply-save-mods]
  genos setup-copy destinations <serverID>
  genos setup-copy start <serverID> <setupID> --destination <serverID> --mode copy|transfer [--wait] [--interval 2s] [--timeout 15m]
  genos setup-copy status <serverID> <copyID>
  genos public-rcon <serverID> [--rotate --yes]
  genos server-order <serverID> [<serverID>...]
  genos plan <serverID>
  genos auth login
  genos auth token
  genos auth status
  genos auth tokens
  genos auth token-create [--name N] [--client-name C] [--machine-name M]
  genos auth token-revoke <tokenID> --yes

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

create-setup POSTs a new profile with --game (use creatable game ids from
genos setups). rename-setup and delete-setup send expectedName and
expectedSelectedSetupID; when --expected-name / --expected are omitted,
genos GETs setups first and uses that setup's name plus selectedSetupID.
delete-setup requires --yes (destructive). rename PATCHes the server name
(API requires confirmed Stopped). broadcast POSTs an in-game notice;
broadcast-status GETs availability/preview. Mutation and broadcast commands
print JSON. Managed files / archive-transfer are not released — use genos
saves for save workflows; there is no files command.

config put reads a JSON object from --file or stdin. Preferred path: the
body already includes expectedSetupID, expectedUpdatedAt, version, and
values (optional secrets). If values is present but any of those three
concurrency fields is omitted, genos GETs configuration first and fills
the missing fields (setupID, updatedAt, version). A body that is not a
JSON object or lacks values fails before any write. API errors such as
server_not_confirmed_stopped are surfaced as-is.

mods list and mutating mods commands print the mods JSON object (pretty),
same spirit as config get. mods search / show print catalog JSON as
returned. Mutating commands send expectedSetupID for compare-and-swap.
When --expected-setup is omitted, genos GETs mods first and uses setupID.
mods apply also autofills stageID from staged.stageID when --stage-id is
omitted (fails clearly if nothing is staged). mods stage requires
--provider (examples: factorio-mod-portal, steam-workshop). API errors
such as server_not_confirmed_stopped and mod_provider_credentials_required
are surfaced as-is. Library prep uses genos profiles mods … (same flags;
on profiles expectedSetupID is the profile id). mods draft replaces the collection draft (--mod-id may repeat; omit for an empty
directModIDs list). mods import reads mod-list.json from --file or stdin.
mods draft-apply autofills expectedSetupID and expectedRevision from GET mods
(collection.draft.revision) when omitted; fails clearly if no draft revision.

saves export starts a server save export (POST …/save-exports). Without
--wait it prints the export JSON immediately. With --wait it polls until
succeeded|failed|expired (default interval 2s, timeout 15m; caps: interval
100ms–1m, timeout ≤24h). Progress lines go to stderr; the final export JSON
goes to stdout. On success the downloadURL is printed to stderr; bytes are
downloaded only when --output is set (never a silent large write). failed
and expired exit non-zero after printing JSON.

profiles lists the account library via dashboard profiles (there is no
GET /profiles list route) and prints profileCapacity. profiles games lists
creatable games. profiles create/rename/delete manage library profiles;
rename/delete autofill expectedName from dashboard when --expected-name is
omitted. delete requires --yes. Attaching a library profile to a running
server is already genos select-setup <serverID> <setupID> (PUT selected-setup);
there is no separate attach route. profiles config get/put mirror server
config put autofill of concurrency fields from GET when omitted.
profiles mods / profiles saves mirror the server mods/saves commands against
/api/v1/profiles/{id}/… for library prep before select-setup attach.

setup-copy destinations/start/status cover setup copy/transfer between owned
servers. --wait polls until succeeded|failed (same style as saves); never
invent success; API errors pass through.

auth tokens lists PAT metadata. auth token-create prints the plaintext secret
once on stdout (never to stderr logs); store it with genos auth token on
stdin. auth token-revoke requires --yes. public-rcon prints the password once (Genos will not show it again); --rotate
requires --yes. server-order must list every owned server exactly once.
plan is read-only (GET …/plan); do not invent plan-changes/reactivate/checkout/
portal follow-ups. Do not auto-ship billing money flows
(plan-changes, reactivate, checkout, portal).

saves import uploads a zip via the dashboard flow: create → PUT uploadURL
(Content-Type application/zip, no Genos bearer) → optional validate/replace.
Without --wait: create+upload then print import JSON. With --wait: validate,
poll to ready (or failed/expired), then require --yes before replace
(destructive); without --yes stop after ready with JSON and an
import-replace hint. With --yes: replace then poll to succeeded|failed|expired
(default timeout 30m). --apply-save-mods / --no-apply-save-mods set
applySaveMods when the review requires a mod choice. recovery is surfaced
on progress lines and in JSON. Library prep uses genos profiles saves …
against /api/v1/profiles/{id}/save-* (same poll/--wait/--output rules).
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
	case "create-setup":
		return runner.createSetup(args[1:])
	case "rename-setup":
		return runner.renameSetup(args[1:])
	case "delete-setup":
		return runner.deleteSetup(args[1:])
	case "rename":
		return runner.rename(args[1:])
	case "broadcast":
		return runner.broadcast(args[1:])
	case "broadcast-status":
		return runner.broadcastStatus(args[1:])
	case "config":
		return runner.config(args[1:])
	case "schema", "management-schema":
		return runner.schema(args[1:])
	case "mods":
		return runner.mods(args[1:])
	case "me":
		return runner.me(args[1:])
	case "dashboard":
		return runner.dashboard(args[1:])
	case "catalog":
		return runner.catalog(args[1:])
	case "profiles":
		return runner.profiles(args[1:])
	case "setup-copy":
		return runner.setupCopy(args[1:])
	case "saves":
		return runner.saves(args[1:])
	case "public-rcon":
		return runner.publicRCON(args[1:])
	case "server-order":
		return runner.serverOrder(args[1:])
	case "plan":
		return runner.plan(args[1:])
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
