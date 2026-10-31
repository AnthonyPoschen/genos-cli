package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func statusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":{"code":"api_code_x","message":"api says no"}}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   ExitCode
	}{
		{"server error 500", http.StatusInternalServerError, ExitFailure},
		{"bad gateway 502", http.StatusBadGateway, ExitFailure},
		{"unauthorized", http.StatusUnauthorized, ExitAuth},
		{"forbidden", http.StatusForbidden, ExitForbidden},
		{"not found", http.StatusNotFound, ExitNotFound},
		{"conflict", http.StatusConflict, ExitConflict},
		{"rate limited", http.StatusTooManyRequests, ExitRateLimited},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := statusServer(t, test.status)
			opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
			if code := Run([]string{"server", "status", "srv-1"}, opts); code != int(test.want) {
				t.Fatalf("exit %d want %d stderr %q", code, test.want, stderr.String())
			}
			if strings.Contains(stderr.String(), testToken) {
				t.Fatal("token leaked")
			}
		})
	}
	t.Run("usage", func(t *testing.T) {
		opts, _, _ := testOptions(t.TempDir(), "http://127.0.0.1:1", testToken, nil)
		if code := Run([]string{"server", "status", "--bogus-flag"}, opts); code != int(ExitUsage) {
			t.Fatalf("exit %d", code)
		}
	})
	t.Run("no token", func(t *testing.T) {
		opts, _, stderr := testOptions(t.TempDir(), "http://127.0.0.1:1", "", nil)
		if code := Run([]string{"server", "status", "srv-1"}, opts); code != int(ExitAuth) {
			t.Fatalf("exit %d stderr %q", code, stderr.String())
		}
	})
	t.Run("network", func(t *testing.T) {
		server := statusServer(t, http.StatusOK)
		url := server.URL
		server.Close()
		opts, _, stderr := testOptions(t.TempDir(), url, testToken, nil)
		if code := Run([]string{"server", "status", "srv-1"}, opts); code != int(ExitNetwork) {
			t.Fatalf("exit %d stderr %q", code, stderr.String())
		}
	})
	t.Run("confirmation required", func(t *testing.T) {
		server := lifecycleServer(t)
		opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
		if code := Run([]string{"server", "stop", "srv-1"}, opts); code != int(ExitConfirmationRequired) {
			t.Fatalf("exit %d stderr %q", code, stderr.String())
		}
	})
}

func lifecycleServer(t *testing.T) *httptest.Server {
	t.Helper()
	body := wrappedServer(0, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/actions") {
			t.Errorf("unexpected action post %s", r.URL.Path)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestJSONErrorShape(t *testing.T) {
	server := statusServer(t, http.StatusNotFound)
	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	code := Run([]string{"me", "--json"}, opts)
	if code != int(ExitNotFound) {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want one stderr line, got %q", stderr.String())
	}
	var got struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("not JSON: %v %q", err, lines[0])
	}
	if got.Error["code"] != "not_found" || got.Error["http_status"] != float64(404) ||
		got.Error["exit_code"] != float64(6) || got.Error["api_code"] != "api_code_x" ||
		!strings.Contains(got.Error["message"].(string), "api says no") {
		t.Fatalf("error %#v", got.Error)
	}
}

func TestJSONUsageError(t *testing.T) {
	opts, _, stderr := testOptions(t.TempDir(), "http://127.0.0.1:1", testToken, nil)
	if code := Run([]string{"me", "--json", "--nope"}, opts); code != int(ExitUsage) {
		t.Fatalf("exit %d", code)
	}
	var got struct {
		Error errorReport `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil || got.Error.Code != "usage" || got.Error.ExitCode != 2 {
		t.Fatalf("stderr %q err %v", stderr.String(), err)
	}
}

func TestTextErrorUnchanged(t *testing.T) {
	server := statusServer(t, http.StatusNotFound)
	opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	Run([]string{"me"}, opts)
	out := stderr.String()
	if !strings.HasPrefix(out, "genos: ") || strings.Contains(out, "{") || strings.Count(out, "\n") != 1 {
		t.Fatalf("stderr %q", out)
	}
}

func TestNoInput(t *testing.T) {
	modes := map[string]func(*Options, []string) []string{
		"flag": func(_ *Options, args []string) []string { return append(args, "--no-input") },
		"env": func(opts *Options, args []string) []string {
			base := opts.Getenv
			opts.Getenv = func(key string) string {
				if key == "GENOS_NONINTERACTIVE" {
					return "1"
				}
				return base(key)
			}
			return args
		},
	}
	for name, apply := range modes {
		t.Run(name+"/confirm", func(t *testing.T) {
			server := lifecycleServer(t)
			opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
			opts.Stdin = strings.NewReader("yes\n")
			opts.IsTerminal = func(io.Reader) bool { return true }
			args := apply(&opts, []string{"server", "force-stop", "srv-1"})
			if code := Run(args, opts); code != int(ExitConfirmationRequired) {
				t.Fatalf("exit %d stderr %q", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "--yes") || strings.Contains(stderr.String(), "type yes") {
				t.Fatalf("stderr %q", stderr.String())
			}
		})
		t.Run(name+"/browser", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/auth/device/codes":
					_, _ = io.WriteString(w, `{"deviceCode":"d","userCode":"ABCD-EFGH","verificationPath":"/device?user_code=ABCD-EFGH","expiresIn":120,"interval":1}`)
				default:
					_, _ = io.WriteString(w, `{"accessToken":"no-input-token-abcdefghijklmnopqrstuvwxyz"}`)
				}
			}))
			defer server.Close()
			opts, stdout, stderr := testOptions(t.TempDir(), server.URL, "", &memoryKeyring{})
			opts.IsTerminal = func(io.Reader) bool { return true }
			opened := false
			opts.OpenURL = func(string) { opened = true }
			args := apply(&opts, []string{"auth", "login"})
			if code := Run(args, opts); code != 0 {
				t.Fatalf("exit %d stderr %q", code, stderr.String())
			}
			if opened {
				t.Fatal("browser opened under no-input")
			}
			if !strings.Contains(stdout.String(), "user code: ABCD-EFGH\n"+server.URL+"/device?user_code=ABCD-EFGH\n") {
				t.Fatalf("stdout %q", stdout.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), "no-input-token") {
				t.Fatal("token leaked")
			}
		})
	}
}

func TestHelpExitCodes(t *testing.T) {
	opts, stdout, _ := testOptions(t.TempDir(), "", "", nil)
	if code := Run([]string{"help", "exit-codes"}, opts); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for code := range exitKinds {
		if !strings.Contains(stdout.String(), fmt.Sprintf("\n  %d  ", code)) {
			t.Fatalf("help missing exit code %d", code)
		}
	}
	if !strings.Contains(stdout.String(), `"exit_code":6`) {
		t.Fatalf("help %q", stdout.String())
	}
}

func modsServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":true}}`)
			return
		}
		_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":false}}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestNoInputSecretReads(t *testing.T) {
	cases := map[string][]string{
		"auth token":       {"auth", "token"},
		"mods credentials": {"server", "mods", "credentials", "srv-1", "--username", "player"},
	}
	for name, args := range cases {
		for _, mode := range []string{"flag", "env"} {
			t.Run(name+"/"+mode+"/tty refuses", func(t *testing.T) {
				server := modsServer(t)
				opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, &memoryKeyring{})
				opts.Stdin = strings.NewReader("typed-secret-value\n")
				opts.IsTerminal = func(io.Reader) bool { return true }
				run := append([]string{}, args...)
				if mode == "flag" {
					run = append(run, "--no-input")
				} else {
					base := opts.Getenv
					opts.Getenv = func(k string) string {
						if k == "GENOS_NONINTERACTIVE" {
							return "true"
						}
						return base(k)
					}
				}
				if code := Run(run, opts); code != int(ExitConfirmationRequired) {
					t.Fatalf("exit %d stderr %q", code, stderr.String())
				}
				if strings.Contains(stderr.String(), "Paste") || !strings.Contains(stderr.String(), "pipe it on stdin") {
					t.Fatalf("stderr %q", stderr.String())
				}
			})
		}
		t.Run(name+"/piped stdin still works", func(t *testing.T) {
			server := modsServer(t)
			opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, &memoryKeyring{})
			opts.Stdin = strings.NewReader("piped-secret-value\n")
			opts.IsTerminal = func(io.Reader) bool { return false }
			if code := Run(append(append([]string{}, args...), "--no-input"), opts); code != 0 {
				t.Fatalf("exit %d stderr %q", code, stderr.String())
			}
		})
	}
}

// TestJSONEveryErrorPath drives error paths on commands that have no --json
// flag of their own, with JSON error rendering switched on.
func TestJSONEveryErrorPath(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		tty   bool
		stdin string
		want  ExitCode
	}{
		{"unknown subcommand", []string{"auth", "bogus"}, false, "", ExitUsage},
		{"unknown help topic", []string{"help", "nope"}, false, "", ExitUsage},
		{"unknown flag", []string{"server", "status", "--nope"}, false, "", ExitUsage},
		{"mods --token refusal", []string{"server", "mods", "credentials", "srv-1", "--token", "flag-secret"}, false, "", ExitUsage},
		{"auth token argument", []string{"auth", "token", "arg-secret"}, false, "", ExitUsage},
		{"confirmation required", []string{"server", "stop", "srv-1"}, false, "", ExitConfirmationRequired},
		{"not confirmed", []string{"server", "stop", "srv-1"}, true, "no\n", ExitFailure},
		{"no-input secret read", []string{"auth", "token", "--no-input"}, true, "x\n", ExitConfirmationRequired},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := lifecycleServer(t)
			opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, &memoryKeyring{})
			opts.Stdin = strings.NewReader(test.stdin)
			opts.IsTerminal = func(io.Reader) bool { return test.tty }
			r := newRunner(opts)
			r.jsonErrors = true
			if code := r.execute(test.args); code != int(test.want) {
				t.Fatalf("exit %d want %d stderr %q", code, test.want, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout %q", stdout.String())
			}
			out := stderr.String()
			if i := strings.LastIndex(out, "[y/N] "); i >= 0 { // the prompt itself precedes the error
				out = out[i+len("[y/N] "):]
			}
			lines := strings.Split(strings.TrimSpace(out), "\n")
			var got struct {
				Error errorReport `json:"error"`
			}
			if len(lines) != 1 || json.Unmarshal([]byte(lines[0]), &got) != nil {
				t.Fatalf("stderr is not one JSON object: %q", stderr.String())
			}
			if got.Error.ExitCode != int(test.want) || got.Error.Code != exitKinds[test.want] || got.Error.Message == "" {
				t.Fatalf("error %+v", got.Error)
			}
			if strings.Contains(stderr.String(), "secret") {
				t.Fatal("secret leaked")
			}
		})
	}
}

func TestJSONFalseIsText(t *testing.T) {
	server := statusServer(t, http.StatusNotFound)
	opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"me", "--json=false"}, opts); code != int(ExitNotFound) {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(stderr.String(), "genos: ") {
		t.Fatalf("stderr %q", stderr.String())
	}
	if !jsonRequested([]string{"--json=true"}) || jsonRequested([]string{"--json", "--json=false"}) || jsonRequested([]string{"--", "--json"}) {
		t.Fatal("jsonRequested")
	}

	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"games":[{"id":"factorio","name":"Factorio"}]}`)
	}))
	defer ok.Close()
	opts, stdout, _ := testOptions(t.TempDir(), ok.URL, testToken, nil)
	if code := Run([]string{"catalog", "--json=false"}, opts); code != 0 || !strings.Contains(stdout.String(), "NAME") || strings.Contains(stdout.String(), "{") {
		t.Fatalf("exit %d stdout %q", code, stdout.String())
	}
}

func TestTextNotConfirmedUnchanged(t *testing.T) {
	server := lifecycleServer(t)
	opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	opts.Stdin = strings.NewReader("no\n")
	opts.IsTerminal = func(io.Reader) bool { return true }
	if code := Run([]string{"server", "stop", "srv-1"}, opts); code != 1 || !strings.HasSuffix(stderr.String(), "[y/N] not confirmed\n") {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
}
