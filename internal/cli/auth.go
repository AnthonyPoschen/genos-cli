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
	if _, err := fmt.Fprintf(r.out, "user code: %s\n%s\n", code.UserCode, approval); err != nil {
		return r.fail(err)
	}
	switch {
	case !r.interactive():
		_, _ = fmt.Fprintln(r.err, "genos: open the URL above to approve; not opening a browser (--no-input)")
	case r.opts.OpenURL != nil:
		r.opts.OpenURL(approval)
	default:
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
		if _, err := fmt.Fprintln(r.err, "Paste access token and press enter."); err != nil {
			return r.fail(err)
		}
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
		if _, err := fmt.Fprintln(r.out, "token saved to keyring"); err != nil {
			return r.fail(err)
		}
	case creds.SourceFile:
		if _, err := fmt.Fprintln(r.out, "token saved to file because no keyring was available"); err != nil {
			return r.fail(err)
		}
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
	resolved, err := creds.ResolveToken(r.env.Token, r.keyring(), creds.File{
		Path:   credentialPath(dir),
		Origin: origin,
	})
	if err != nil {
		if errors.Is(err, creds.ErrNotFound) {
			return r.fail(noTokenError(origin))
		}
		return r.fail(err)
	}
	if _, err := fmt.Fprintf(r.out, "origin: %s\nsource: %s\nprefix: %s\n", origin, resolved.Source, creds.Prefix(resolved.Token)); err != nil {
		return r.fail(err)
	}
	return 0
}

func (r *runner) authTokens(asJSON bool) int {
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tokens, err := client.ListAccessTokens(ctx)
	if err != nil {
		return r.fail(err)
	}
	if asJSON {
		return r.printJSON(map[string]any{"tokens": tokens})
	}
	if len(tokens) == 0 {
		WriteEmpty(r.out, "No tokens.")
		return 0
	}
	rows := make([][]string, 0, len(tokens))
	for _, token := range tokens {
		rows = append(rows, []string{
			token.ID,
			token.TokenPrefix,
			token.Name,
			token.ClientName,
			token.MachineName,
			formatDate(token.CreatedAt),
			formatDatePtr(token.LastUsedAt),
			formatExpiry(token.ExpiresAt, time.Now()),
			formatDatePtr(token.RevokedAt),
		})
	}
	WriteTable(r.out, []string{"ID", "PREFIX", "NAME", "CLIENT", "MACHINE", "CREATED", "LAST USED", "EXPIRES", "REVOKED"}, rows)
	return 0
}

// formatDate renders a timestamp as a calendar day. The API has no scope
// field on PATs; PREFIX is the non-secret identifier shown beside the name.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatDate(*t)
}

// formatExpiry prints "never" when a token does not expire, "in Nd" while it
// is still valid, and the calendar day once it has passed.
func formatExpiry(t *time.Time, now time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	exp := t.UTC()
	if exp.After(now.UTC()) {
		days := int(exp.Sub(now.UTC()).Hours() / 24)
		if days < 1 {
			return "in <1d"
		}
		return fmt.Sprintf("in %dd", days)
	}
	return exp.Format("2006-01-02")
}

func (r *runner) authTokenCreate(args []string) int {
	rest, flags, err := splitControlFlags(args, map[string]bool{
		"--name": true, "--client-name": true, "--machine-name": true,
	}, nil)
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 0 {
		return r.usage("auth token-create takes no positional arguments")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := client.CreateAccessToken(ctx, flags["--name"], flags["--client-name"], flags["--machine-name"])
	if err != nil {
		return r.fail(err)
	}
	// Print the full creation payload once on stdout (includes secret).
	// Do not write the secret to stderr. Prefer storing via: genos auth token <secret.
	if code := r.printJSON(result); code != 0 {
		return code
	}
	if _, err := fmt.Fprintln(r.err, "genos: secret printed once above; store with: genos auth token  (stdin)"); err != nil {
		return r.fail(err)
	}
	return 0
}

func (r *runner) authTokenRevoke(args []string) int {
	rest, _, switches, err := splitControlFlagsWithSwitches(args, nil, map[string]bool{"--yes": true})
	if errors.Is(err, errHelp) {
		return r.usage("")
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("auth token-revoke requires a token id and --yes")
	}
	if !switches["--yes"] {
		return r.usage("auth token-revoke requires --yes")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(fmt.Errorf("invalid token id %q", rest[0]))
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.RevokeAccessToken(ctx, rest[0]); err != nil {
		return r.fail(err)
	}
	if _, err := fmt.Fprintf(r.out, "revoked %s\n", rest[0]); err != nil {
		return r.fail(err)
	}
	return 0
}
