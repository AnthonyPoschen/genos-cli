package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

// ExitCode is a documented, stable process exit status. Scripts and agents
// may branch on these values; never renumber an existing code.
type ExitCode int

const (
	ExitOK                   ExitCode = 0
	ExitFailure              ExitCode = 1
	ExitUsage                ExitCode = 2
	ExitConfirmationRequired ExitCode = 3
	ExitAuth                 ExitCode = 4
	ExitForbidden            ExitCode = 5
	ExitNotFound             ExitCode = 6
	ExitConflict             ExitCode = 7
	ExitRateLimited          ExitCode = 8
	ExitNetwork              ExitCode = 9
)

// exitKinds are the machine-readable error codes in a --json error, one per exit code.
var exitKinds = map[ExitCode]string{
	ExitFailure:              "failure",
	ExitUsage:                "usage",
	ExitConfirmationRequired: "confirmation_required",
	ExitAuth:                 "auth",
	ExitForbidden:            "forbidden",
	ExitNotFound:             "not_found",
	ExitConflict:             "conflict",
	ExitRateLimited:          "rate_limited",
	ExitNetwork:              "network",
}

const exitCodesHelp = `Exit codes are stable; scripts and agents may branch on them.

  0  success
  1  generic failure (API 5xx, validation, a declined prompt, anything else)
  2  usage error (bad arguments or flags, missing required --yes)
  3  confirmation required: a prompt was needed but input is not interactive; re-run with --yes
  4  authentication: no token, or the API answered 401
  5  forbidden: the API answered 403
  6  not found: the API answered 404
  7  conflict: the API answered 409 or 412 (state changed; re-read and retry)
  8  rate limited: the API answered 429
  9  network: the Genos API could not be reached

With --json, a failure also prints one JSON object on stderr:

  {"error":{"code":"not_found","message":"...","http_status":404,"api_code":"...","hint":"...","exit_code":6}}

code is the name for exit_code above. http_status and api_code appear only for API
errors; hint only when there is one. Without --json, errors stay "genos: <message>".
`

// cliError carries an exit code and optional hint through command code.
type cliError struct {
	code ExitCode
	msg  string
	hint string
}

func (e *cliError) Error() string { return e.msg }

func noTokenError(origin string) error {
	return &cliError{code: ExitAuth, msg: "no token for " + origin, hint: "set GENOS_TOKEN or run genos auth token"}
}

type errorReport struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
	APICode    string `json:"api_code,omitempty"`
	Hint       string `json:"hint,omitempty"`
	ExitCode   int    `json:"exit_code"`
}

func classify(err error) errorReport {
	report := errorReport{Message: err.Error(), ExitCode: int(ExitFailure)}
	var ce *cliError
	var api *apiclient.APIError
	var urlErr *url.Error
	var netErr net.Error
	switch {
	case errors.As(err, &ce):
		report.ExitCode, report.Hint = int(ce.code), ce.hint
	case errors.As(err, &api):
		report.HTTPStatus, report.APICode = api.Status, api.Code
		switch api.Status {
		case http.StatusUnauthorized:
			report.ExitCode, report.Hint = int(ExitAuth), "set GENOS_TOKEN or run genos auth token"
		case http.StatusForbidden:
			report.ExitCode = int(ExitForbidden)
		case http.StatusNotFound:
			report.ExitCode = int(ExitNotFound)
		case http.StatusConflict, http.StatusPreconditionFailed:
			report.ExitCode, report.Hint = int(ExitConflict), "re-read the current state and retry"
		case http.StatusTooManyRequests:
			report.ExitCode, report.Hint = int(ExitRateLimited), "wait and retry"
		}
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &urlErr), errors.As(err, &netErr):
		report.ExitCode, report.Hint = int(ExitNetwork), "check the network and GENOS_HOST"
	}
	report.Code = exitKinds[ExitCode(report.ExitCode)]
	return report
}

// report prints a failure in text or JSON form and returns its exit code.
func (r *runner) report(rep errorReport) int {
	if r.jsonErrors {
		data, _ := json.Marshal(struct {
			Error errorReport `json:"error"`
		}{rep})
		_, _ = fmt.Fprintln(r.err, string(data))
		return rep.ExitCode
	}
	if rep.Message != "" {
		_, _ = fmt.Fprintf(r.err, "genos: %s\n", rep.Message)
	}
	return rep.ExitCode
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
