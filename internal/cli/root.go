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
	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/cobra"
)

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

// Env holds GENOS_* settings loaded via envconfig (prefix GENOS).
type Env struct {
	Host  string `envconfig:"HOST"`
	Token string `envconfig:"TOKEN"`
}

// Execute runs the Cobra command tree with args (without argv0) and returns an exit code.
func Execute(args []string, opts Options) int {
	runner := newRunner(opts)
	cmd := newRootCmd(runner)
	cmd.SetArgs(args)
	cmd.SetIn(runner.in)
	cmd.SetOut(runner.out)
	cmd.SetErr(runner.err)
	if err := cmd.Execute(); err != nil {
		var exit *exitError
		if errors.As(err, &exit) {
			return exit.code
		}
		fmt.Fprintf(runner.err, "genos: %s\n", err)
		return 2
	}
	return 0
}

// Run delegates to Execute (kept for existing call sites and tests).
func Run(args []string, opts Options) int {
	return Execute(args, opts)
}

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit %d", e.code)
}

func codeErr(code int) error {
	if code == 0 {
		return nil
	}
	return &exitError{code: code}
}

func newRunner(opts Options) *runner {
	r := &runner{opts: opts, in: opts.Stdin, out: opts.Stdout, err: opts.Stderr}
	if r.in == nil {
		r.in = os.Stdin
	}
	if r.out == nil {
		r.out = os.Stdout
	}
	if r.err == nil {
		r.err = os.Stderr
	}
	r.env = r.loadEnv()
	return r
}

func (r *runner) loadEnv() Env {
	if r.opts.Getenv != nil {
		return Env{
			Host:  r.opts.Getenv("GENOS_HOST"),
			Token: r.opts.Getenv("GENOS_TOKEN"),
		}
	}
	var env Env
	_ = envconfig.Process("GENOS", &env)
	return env
}

type runner struct {
	opts Options
	in   io.Reader
	out  io.Writer
	err  io.Writer
	env  Env
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
	origin, err = config.ResolveOrigin(r.env.Host, filepath.Join(dir, "config.toml"))
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

func newRootCmd(r *runner) *cobra.Command {
	root := &cobra.Command{
		Use:           "genos",
		Short:         "Customer CLI for Genos game servers",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = cmd.Help()
			return &exitError{code: 2}
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	root.AddCommand(
		newServerCmd(r),
		newProfileCmd(r),
		newRconCmd(r),
		newAuthCmd(r),
		newCatalogCmd(r),
		newMeCmd(r),
		newDashboardCmd(r),
		newSchemaCmd(r),
	)
	return root
}

func appendFlag(args []string, name, value string, set bool) []string {
	if !set {
		return args
	}
	return append(args, name, value)
}

func appendSwitch(args []string, name string, set bool) []string {
	if !set {
		return args
	}
	return append(args, name)
}

func flagChanged(cmd *cobra.Command, name string) bool {
	f := cmd.Flags().Lookup(name)
	return f != nil && f.Changed
}

var errHelp = errors.New("help")

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

// markContainer makes a parent reject unknown leftover args with exit 2,
// and show help (exit 0) when invoked with no subcommand.
func markContainer(cmd *cobra.Command) *cobra.Command {
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			fmt.Fprintf(c.ErrOrStderr(), "genos: unknown command %q for %q\n", args[0], c.CommandPath())
			return &exitError{code: 2}
		}
		_ = c.Help()
		return nil
	}
	return cmd
}
