package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
	"github.com/AnthonyPoschen/genos-cli/internal/creds"
)

func (r *runner) auth(args []string) int {
	if len(args) == 0 {
		return r.usage("auth requires login, token, or status")
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Fprint(r.out, usage)
		return 0
	case "login":
		if len(args) != 1 {
			return r.usage("auth login takes no arguments")
		}
		return r.login()
	case "token":
		if len(args) != 1 {
			fmt.Fprintln(r.err, "genos: read the token from stdin; do not pass it as an argument")
			return 2
		}
		return r.storeStdin()
	case "status":
		if len(args) != 1 {
			return r.usage("auth status takes no arguments")
		}
		return r.authStatus()
	default:
		return r.usage("unknown auth command " + args[0])
	}
}

func (r *runner) login() int {
	origin, dir, err := r.origin()
	if err != nil {
		return r.fail(err)
	}
	machine, err := os.Hostname()
	if err != nil || strings.TrimSpace(machine) == "" {
		machine = "unknown"
	}
	client := apiclient.New(origin, "")
	ctx := context.Background()
	code, err := client.RequestDeviceCode(ctx, machine)
	if err != nil {
		return r.fail(err)
	}
	approval, err := apiclient.ApprovalURL(origin, code.VerificationPath)
	if err != nil {
		return r.fail(err)
	}
	fmt.Fprintf(r.out, "user code: %s\n%s\n", code.UserCode, approval)
	if r.opts.OpenURL != nil {
		r.opts.OpenURL(approval)
	} else {
		openBrowser(approval)
	}
	interval := time.Duration(code.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(15 * time.Minute)
	if !code.ExpiresAt.IsZero() {
		deadline = code.ExpiresAt
	} else if code.ExpiresIn > 0 {
		deadline = time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)
	}
	token, err := client.PollDeviceToken(ctx, code.DeviceCode, interval, deadline, r.opts.Sleep)
	if err != nil {
		return r.fail(err)
	}
	return r.storeToken(origin, dir, token)
}

func (r *runner) storeStdin() int {
	if r.terminal() {
		fmt.Fprintln(r.err, "Paste access token and press enter.")
	}
	payload, err := io.ReadAll(io.LimitReader(r.in, 16<<10+1))
	if err != nil {
		return r.fail(err)
	}
	if len(payload) > 16<<10 {
		return r.fail(errors.New("token is too large"))
	}
	token := strings.TrimSpace(string(payload))
	if token == "" {
		return r.fail(errors.New("token is empty"))
	}
	origin, dir, err := r.origin()
	if err != nil {
		return r.fail(err)
	}
	return r.storeToken(origin, dir, token)
}

func (r *runner) storeToken(origin, dir, token string) int {
	source, err := creds.Save(origin, token, r.keyring(), credentialPath(dir))
	if err != nil {
		return r.fail(err)
	}
	switch source {
	case creds.SourceKeyring:
		fmt.Fprintln(r.out, "token saved to keyring")
	case creds.SourceFile:
		fmt.Fprintln(r.out, "token saved to file because no keyring was available")
	default:
		return r.fail(fmt.Errorf("unknown credential store %q", source))
	}
	return 0
}

func (r *runner) authStatus() int {
	origin, dir, err := r.origin()
	if err != nil {
		return r.fail(err)
	}
	resolved, err := creds.ResolveToken(r.getenv("GENOS_TOKEN"), r.keyring(), creds.File{
		Path:   credentialPath(dir),
		Origin: origin,
	})
	if err != nil {
		if errors.Is(err, creds.ErrNotFound) {
			return r.fail(fmt.Errorf("no token for %s", origin))
		}
		return r.fail(err)
	}
	fmt.Fprintf(r.out, "origin: %s\nsource: %s\nprefix: %s\n", origin, resolved.Source, creds.Prefix(resolved.Token))
	return 0
}
