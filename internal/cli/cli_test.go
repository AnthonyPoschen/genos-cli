package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/config"
	"github.com/AnthonyPoschen/genos-cli/internal/creds"
)

const testToken = "abcdefghijklmnopqrstuvwxyz"

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type hit struct {
	Method string
	Path   string
	Auth   string
	Key    string
	Body   string
}

type recorder struct {
	mu   sync.Mutex
	hits []hit
}

func (r *recorder) Handler(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		payload, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.hits = append(r.hits, hit{
			Method: req.Method,
			Path:   req.URL.Path,
			Auth:   req.Header.Get("Authorization"),
			Key:    req.Header.Get("Idempotency-Key"),
			Body:   string(payload),
		})
		r.mu.Unlock()
		next(w, req)
	})
}

func (r *recorder) snapshot() []hit {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]hit, len(r.hits))
	copy(out, r.hits)
	return out
}

type memoryKeyring struct {
	token  string
	origin string
	getErr error
	setErr error
	gets   int
}

func (m *memoryKeyring) Get(origin string) (string, error) {
	m.gets++
	m.origin = origin
	if m.getErr != nil {
		return "", m.getErr
	}
	return m.token, nil
}

func (m *memoryKeyring) Set(origin, secret string) error {
	m.origin = origin
	if m.setErr != nil {
		return m.setErr
	}
	m.token = secret
	return nil
}

func testOptions(xdg, host, token string, ring creds.Keyring) (Options, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if ring == nil {
		ring = &memoryKeyring{getErr: errors.New("keyring unused")}
	}
	opts := Options{
		Stdin:  &bytes.Buffer{},
		Stdout: stdout,
		Stderr: stderr,
		Getenv: func(key string) string {
			switch key {
			case "GENOS_HOST":
				return host
			case "GENOS_TOKEN":
				return token
			case "XDG_CONFIG_HOME":
				return xdg
			default:
				return ""
			}
		},
		Keyring: func() creds.Keyring { return ring },
		OpenURL: func(string) {},
		Sleep:   func(time.Duration) {},
	}
	return opts, stdout, stderr
}

func TestServersAndStatus(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/servers":
			_, _ = io.WriteString(w, `{"servers":[{"id":"srv-1","name":"Alpha","game":{"name":"Factorio"},"status":"Running"},{"id":"srv-2","name":"Beta","game":{"name":"Rust"},"status":"Stopped"}]}`)
		case "/api/v1/servers/srv-1":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Alpha","game":{"name":"Factorio"},"status":"Running"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	xdg := t.TempDir()
	opts, stdout, stderr := testOptions(xdg, server.URL, testToken, nil)
	if code := Run([]string{"servers"}, opts); code != 0 {
		t.Fatalf("servers exit %d stderr %s", code, stderr.String())
	}
	if stdout.String() != "Alpha\tFactorio\tRunning\nBeta\tRust\tStopped\n" {
		t.Fatalf("servers output %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Auth != "Bearer "+testToken || hits[0].Path != "/api/v1/servers" {
		t.Fatalf("list request %+v", hits)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "srv-1"}, opts); code != 0 {
		t.Fatalf("status exit %d stderr %s", code, stderr.String())
	}
	if stdout.String() != "Alpha\tFactorio\tRunning\n" {
		t.Fatalf("status output %q", stdout.String())
	}
}

func TestLifecycleConfirmation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		players     int
		updates     []string
		stdin       string
		tty         bool
		wantPost    bool
		wantConfirm bool
	}{
		{name: "start", args: []string{"start", "srv-1"}, players: 4, updates: []string{"build"}, wantPost: true},
		{name: "stop without yes", args: []string{"stop", "srv-1"}},
		{name: "stop yes", args: []string{"stop", "--yes", "srv-1"}, wantPost: true},
		{name: "stop yes after id", args: []string{"stop", "srv-1", "--yes"}, wantPost: true},
		{name: "force-stop without yes", args: []string{"force-stop", "srv-1"}},
		{name: "force-stop yes", args: []string{"force-stop", "srv-1", "--yes"}, wantPost: true, wantConfirm: true},
		{name: "force-stop tty yes", args: []string{"force-stop", "srv-1"}, stdin: "yes\n", tty: true, wantPost: true, wantConfirm: true},
		{name: "stop tty no", args: []string{"stop", "srv-1"}, stdin: "no\n", tty: true},
		{name: "restart idle", args: []string{"restart", "srv-1"}, wantPost: true},
		{name: "restart players", args: []string{"restart", "srv-1"}, players: 2},
		{name: "restart updates", args: []string{"restart", "srv-1"}, updates: []string{"mod"}},
		{name: "restart busy yes", args: []string{"restart", "--yes", "srv-1"}, players: 2, updates: []string{"mod"}, wantPost: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := &recorder{}
			body := wrappedServer(test.players, test.updates)
			server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/actions") {
					w.WriteHeader(http.StatusAccepted)
					_, _ = io.WriteString(w, `{"disposition":"accepted"}`)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()

			opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
			opts.Stdin = strings.NewReader(test.stdin)
			opts.IsTerminal = func(io.Reader) bool { return test.tty }
			code := Run(test.args, opts)
			hits := rec.snapshot()
			var posts []hit
			for _, item := range hits {
				if strings.HasSuffix(item.Path, "/actions") {
					posts = append(posts, item)
				}
			}
			if test.wantPost {
				if code != 0 {
					t.Fatalf("exit %d stderr %s", code, stderr.String())
				}
				if len(posts) != 1 {
					t.Fatalf("posts %+v all %+v", posts, hits)
				}
				post := posts[0]
				if post.Method != http.MethodPost || post.Auth != "Bearer "+testToken {
					t.Fatalf("post %+v", post)
				}
				if !uuidPattern.MatchString(post.Key) {
					t.Fatalf("idempotency key %q", post.Key)
				}
				if !strings.Contains(post.Body, `"type":"`+test.args[0]+`"`) {
					t.Fatalf("body %s", post.Body)
				}
				flag := `"confirmUnsavedProgressLoss":false`
				if test.wantConfirm {
					flag = `"confirmUnsavedProgressLoss":true`
				}
				if !strings.Contains(post.Body, flag) {
					t.Fatalf("body %s, want %s", post.Body, flag)
				}
				if !strings.Contains(stdout.String(), "requested "+test.args[0]+" for Alpha") {
					t.Fatalf("stdout %q", stdout.String())
				}
				return
			}
			if code == 0 || len(posts) != 0 {
				t.Fatalf("exit %d posts %+v stdout %q stderr %q", code, posts, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), testToken) || strings.Contains(stderr.String(), testToken) {
				t.Fatal("token leaked")
			}
			if test.args[0] == "force-stop" && !strings.Contains(stderr.String(), "unsaved progress") {
				t.Fatalf("stderr %q", stderr.String())
			}
			if test.args[0] == "stop" && !strings.Contains(stderr.String(), "Stop Alpha?") {
				t.Fatalf("stderr %q", stderr.String())
			}
		})
	}
}

func TestConsole(t *testing.T) {
	t.Run("interactive", func(t *testing.T) {
		rec := &recorder{}
		server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/commands") {
				_, _ = io.WriteString(w, `{"response":"players: 1"}`)
				return
			}
			_, _ = io.WriteString(w, `{"channels":[{"id":"container-stdout","interaction":"read-only"},{"id":"rcon","interaction":"interactive"}]}`)
		}))
		defer server.Close()
		opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
		if code := Run([]string{"console", "srv-1", "say", "hello"}, opts); code != 0 {
			t.Fatalf("exit %d stderr %s", code, stderr.String())
		}
		if stdout.String() != "players: 1\n" {
			t.Fatalf("stdout %q", stdout.String())
		}
		hits := rec.snapshot()
		if len(hits) != 2 || !strings.HasSuffix(hits[1].Path, "/runtime-channels/rcon/commands") {
			t.Fatalf("hits %+v", hits)
		}
		if !strings.Contains(hits[1].Body, `"text":"say hello"`) || hits[1].Auth != "Bearer "+testToken {
			t.Fatalf("command %+v", hits[1])
		}
	})

	t.Run("no interactive channel", func(t *testing.T) {
		rec := &recorder{}
		server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"channels":[{"id":"container-stdout","interaction":"read-only"}]}`)
		}))
		defer server.Close()
		opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
		if code := Run([]string{"console", "srv-1", "hello"}, opts); code == 0 {
			t.Fatal("expected error")
		}
		if !strings.Contains(stderr.String(), "no interactive console channel") {
			t.Fatalf("stderr %q", stderr.String())
		}
		for _, item := range rec.snapshot() {
			if strings.HasSuffix(item.Path, "/commands") {
				t.Fatal("command was sent")
			}
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout %q", stdout.String())
		}
	})
}

func TestAuthStatus(t *testing.T) {
	xdg := t.TempDir()
	opts, stdout, stderr := testOptions(xdg, "https://genosservers.com", testToken, nil)
	if code := Run([]string{"auth", "status"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "origin: https://genosservers.com\n") || !strings.Contains(got, "source: env\n") || !strings.Contains(got, "prefix: abcdefghijklmnop\n") {
		t.Fatalf("status %q", got)
	}
	if strings.Contains(got, "qrstuvwxyz") || strings.Contains(stderr.String(), testToken) {
		t.Fatal("full token printed")
	}

	ring := &memoryKeyring{token: testToken}
	opts, stdout, stderr = testOptions(xdg, "https://genosservers.com", "", ring)
	if code := Run([]string{"auth", "status"}, opts); code != 0 {
		t.Fatalf("keyring status exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "source: keyring\n") || ring.origin != "https://genosservers.com" || ring.gets != 1 {
		t.Fatalf("status %q ring %+v", stdout.String(), ring)
	}
	if strings.Contains(stdout.String(), "qrstuvwxyz") {
		t.Fatal("full keyring token printed")
	}

	path := filepath.Join(config.Dir(xdg, ""), "credentials.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTokenFile(t, path, "https://genosservers.com", testToken, 0o600)
	opts, stdout, stderr = testOptions(xdg, "https://genosservers.com", "", &memoryKeyring{getErr: errors.New("miss")})
	if code := Run([]string{"auth", "status"}, opts); code != 0 {
		t.Fatalf("file status exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "source: file\n") || strings.Contains(stdout.String(), "qrstuvwxyz") {
		t.Fatalf("status %q", stdout.String())
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"auth", "status"}, opts); code == 0 || !strings.Contains(stderr.String(), "chmod 0600") {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "abcdefghijklmnop") || strings.Contains(stderr.String(), testToken) {
		t.Fatal("loose file token was used")
	}
}

func TestAuthLogin(t *testing.T) {
	const accessToken = "login-access-token-abcdefghijklmnopqrstuvwxyz"
	var opened string
	var polls int
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/device/codes":
			_, _ = io.WriteString(w, `{"deviceCode":"device-1","userCode":"ABCD-EFGH","verificationPath":"/device?user_code=ABCD-EFGH","expiresIn":120,"interval":1}`)
		case "/api/v1/auth/device/tokens":
			polls++
			if polls > 4 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"code":"expired","message":"too many polls"}}`)
				return
			}
			if polls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"code":"authorization_pending","message":"wait"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"accessToken":"`+accessToken+`"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Run("keyring", func(t *testing.T) {
		polls = 0
		xdg := t.TempDir()
		ring := &memoryKeyring{}
		opts, stdout, stderr := testOptions(xdg, server.URL, "", ring)
		opts.OpenURL = func(rawURL string) { opened = rawURL }
		if code := Run([]string{"auth", "login"}, opts); code != 0 {
			t.Fatalf("exit %d stderr %s", code, stderr.String())
		}
		wantURL := server.URL + "/device?user_code=ABCD-EFGH"
		if opened != wantURL || !strings.Contains(stdout.String(), "user code: ABCD-EFGH\n"+wantURL+"\n") {
			t.Fatalf("opened %q stdout %q", opened, stdout.String())
		}
		if strings.Contains(opened, accessToken) || strings.Contains(stdout.String(), accessToken) || strings.Contains(stderr.String(), accessToken) {
			t.Fatal("access token leaked into approval output")
		}
		if !strings.Contains(stdout.String(), "token saved to keyring") || strings.Contains(stdout.String(), "saved to file") {
			t.Fatalf("stdout %q", stdout.String())
		}
		if ring.token != accessToken || ring.origin != server.URL {
			t.Fatalf("ring %+v", ring)
		}
		var codeBody map[string]string
		var sawCode bool
		var sawPoll bool
		for _, item := range rec.snapshot() {
			switch item.Path {
			case "/api/v1/auth/device/codes":
				sawCode = true
				if item.Method != http.MethodPost {
					t.Fatalf("code method %s", item.Method)
				}
				if err := json.Unmarshal([]byte(item.Body), &codeBody); err != nil {
					t.Fatal(err)
				}
			case "/api/v1/auth/device/tokens":
				sawPoll = true
				if !strings.Contains(item.Body, `"deviceCode":"device-1"`) {
					t.Fatalf("poll body %s", item.Body)
				}
			}
		}
		if !sawCode || !sawPoll || codeBody["clientName"] != "genos-cli" || codeBody["host"] != server.URL || codeBody["machineName"] == "" {
			t.Fatalf("device requests code %#v saw %v %v", codeBody, sawCode, sawPoll)
		}
		if _, err := os.Stat(filepath.Join(xdg, "genos", "credentials.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("credentials file exists: %v", err)
		}
		for _, item := range rec.snapshot() {
			if item.Auth != "" {
				t.Fatalf("device call sent authorization %q", item.Auth)
			}
		}
	})

	t.Run("file fallback", func(t *testing.T) {
		polls = 0
		rec.mu.Lock()
		rec.hits = nil
		rec.mu.Unlock()
		xdg := t.TempDir()
		ring := &memoryKeyring{setErr: errors.New("no keyring")}
		opts, stdout, stderr := testOptions(xdg, server.URL, "", ring)
		opts.OpenURL = func(string) {}
		if code := Run([]string{"auth", "login"}, opts); code != 0 {
			t.Fatalf("exit %d stderr %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "token saved to file because no keyring was available") {
			t.Fatalf("stdout %q", stdout.String())
		}
		if strings.Contains(stdout.String(), accessToken) || strings.Contains(stderr.String(), accessToken) {
			t.Fatal("token leaked")
		}
		path := filepath.Join(xdg, "genos", "credentials.json")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("file mode %o", info.Mode().Perm())
		}
		dirInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode %o", dirInfo.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), accessToken) {
			t.Fatalf("file %s", data)
		}
	})
}

func TestAuthToken(t *testing.T) {
	xdg := t.TempDir()
	ring := &memoryKeyring{setErr: errors.New("no keyring")}
	opts, stdout, stderr := testOptions(xdg, "https://genosservers.com", "", ring)
	opts.Stdin = strings.NewReader("  " + testToken + "\n")
	if code := Run([]string{"auth", "token"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "token saved to file because no keyring was available") {
		t.Fatalf("stdout %q", stdout.String())
	}
	if strings.Contains(stdout.String(), testToken) || strings.Contains(stderr.String(), testToken) {
		t.Fatal("stdin token was printed")
	}
	data, err := os.ReadFile(filepath.Join(xdg, "genos", "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), testToken) {
		t.Fatalf("file %s", data)
	}

	secret := "flag-secret-should-not-be-stored"
	rejected := &memoryKeyring{}
	opts, stdout, stderr = testOptions(t.TempDir(), "https://genosservers.com", "", rejected)
	if code := Run([]string{"auth", "token", secret}, opts); code == 0 {
		t.Fatal("accepted token argument")
	}
	if rejected.token != "" {
		t.Fatal("stored the argument")
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatal("argument secret was echoed")
	}
}

func TestRejectsPublicHTTPAndIgnoresLocalEnv(t *testing.T) {
	xdg := t.TempDir()
	dir := config.Dir(xdg, "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "local-env-secret-should-not-leak"
	if err := os.WriteFile(filepath.Join(dir, "local.env"), []byte("GENOS_TOKEN="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, stdout, stderr := testOptions(xdg, "http://example.com", "", &memoryKeyring{getErr: errors.New("miss")})
	if code := Run([]string{"servers"}, opts); code == 0 || !strings.Contains(stderr.String(), "https") {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), secret) {
		t.Fatal("local.env leaked")
	}

	opts, stdout, stderr = testOptions(xdg, "", "", &memoryKeyring{getErr: errors.New("miss")})
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("currentHost = \"https://genosservers.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"auth", "status"}, opts); code == 0 {
		t.Fatal("status succeeded without a token")
	}
	if strings.Contains(stdout.String()+stderr.String(), secret) {
		t.Fatal("local.env leaked")
	}
}

func TestUsage(t *testing.T) {
	opts, stdout, stderr := testOptions(t.TempDir(), "", "", nil)
	if code := Run(nil, opts); code != 2 || !strings.Contains(stderr.String(), "genos servers") {
		t.Fatalf("no args exit %d stderr %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"help"}, opts); code != 0 || !strings.Contains(stdout.String(), "genos auth token") {
		t.Fatalf("help exit %d stdout %q", code, stdout.String())
	}
	if code := Run([]string{"nope"}, opts); code != 2 {
		t.Fatalf("unknown exit %d", code)
	}
}

func wrappedServer(players int, updates []string) string {
	if updates == nil {
		updates = []string{}
	}
	payload := map[string]any{
		"server": map[string]any{
			"id":             "srv-1",
			"name":           "Alpha",
			"game":           map[string]any{"name": "Factorio"},
			"status":         "Running",
			"notableUpdates": updates,
			"metrics":        map[string]any{"playerCount": players},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func writeTokenFile(t *testing.T, path, host, token string, mode os.FileMode) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"hosts": map[string]any{
			host: map[string]string{"token": token},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
