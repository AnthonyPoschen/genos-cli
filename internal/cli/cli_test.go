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

func TestSetupsList(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/servers/srv-1/setups" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}},{"id":"setup-b","name":"Alt","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a","capacity":{"used":2,"limit":5},"creatableGames":[]}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"setups", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	want := "setup-a\tMain\tFactorio\t*\nsetup-b\tAlt\tFactorio\t\n"
	if stdout.String() != want {
		t.Fatalf("stdout %q want %q", stdout.String(), want)
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Auth != "Bearer "+testToken || hits[0].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
}

func TestSelectSetupDefaultsExpectedFromChooser(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setups":
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}},{"id":"setup-b","name":"Alt","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/selected-setup":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","selectedSetupID":"setup-b","selectedSetupName":"Alt"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"select-setup", "srv-1", "setup-b"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if stdout.String() != "selected setup-b for srv-1\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 2 {
		t.Fatalf("hits %+v", hits)
	}
	if hits[0].Method != http.MethodGet || hits[0].Path != "/api/v1/servers/srv-1/setups" {
		t.Fatalf("first hit %+v", hits[0])
	}
	put := hits[1]
	if put.Method != http.MethodPut || put.Path != "/api/v1/servers/srv-1/selected-setup" {
		t.Fatalf("put %+v", put)
	}
	if put.Key != "" {
		t.Fatalf("unexpected idempotency key %q", put.Key)
	}
	if put.Auth != "Bearer "+testToken {
		t.Fatalf("auth %q", put.Auth)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(put.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["setupID"] != "setup-b" || body["expectedSelectedSetupID"] != "setup-a" {
		t.Fatalf("body %#v", body)
	}
}

func TestSelectSetupExplicitExpectedSkipsChooserGET(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/selected-setup" {
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","selectedSetupID":"setup-b"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"select-setup", "srv-1", "setup-b", "--expected", "setup-a"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "selected setup-b") {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Method != http.MethodPut {
		t.Fatalf("hits %+v", hits)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(hits[0].Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["expectedSelectedSetupID"] != "setup-a" {
		t.Fatalf("body %#v", body)
	}
}

func TestSelectSetupRunningSurfacesAPIError(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setups":
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/selected-setup":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"the Server must be confirmed stopped before changing Profiles"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"select-setup", "srv-1", "setup-b"}, opts); code == 0 {
		t.Fatal("expected failure while Running")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	errText := stderr.String()
	if !strings.Contains(errText, "server_not_confirmed_stopped") {
		t.Fatalf("stderr %q missing server_not_confirmed_stopped", errText)
	}
	hits := rec.snapshot()
	var putCount int
	for _, item := range hits {
		if item.Method == http.MethodPut {
			putCount++
		}
	}
	if putCount != 1 {
		t.Fatalf("expected one PUT, hits %+v", hits)
	}
}

func TestUnloadSetupDefaultsExpected(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setups":
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/servers/srv-1/selected-setup":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","selectedSetupID":""}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"unload-setup", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if stdout.String() != "unloaded setup for srv-1\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 2 || hits[1].Method != http.MethodDelete || hits[1].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(hits[1].Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["expectedSelectedSetupID"] != "setup-a" {
		t.Fatalf("body %#v", body)
	}
}

func TestUnloadSetupEmptySelectedDefaultsEmptyExpected(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/setups"):
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}}]}`)
		case r.Method == http.MethodDelete:
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"unload-setup", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	hits := rec.snapshot()
	var body map[string]string
	if err := json.Unmarshal([]byte(hits[1].Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["expectedSelectedSetupID"] != "" {
		t.Fatalf("body %#v", body)
	}
}

func TestExpectedFlagRequiresValue(t *testing.T) {
	opts, _, stderr := testOptions(t.TempDir(), "https://genosservers.com", testToken, nil)
	if code := Run([]string{"select-setup", "srv-1", "setup-b", "--expected"}, opts); code != 2 {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--expected requires a value") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestConfigGet(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/servers/srv-1/configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","name":"Main","gameID":"factorio","version":"v1","values":{"name":"Alpha"},"editable":true,"updatedAt":"2026-01-02T03:04:05Z","secrets":{"version":"s1","configured":true,"pending":false}}}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"config", "get", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"setupID": "setup-a"`) || !strings.Contains(out, `"version": "v1"`) {
		t.Fatalf("stdout %q", out)
	}
	if strings.Contains(out, `"configuration"`) {
		t.Fatalf("should print inner configuration object, got %q", out)
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Auth != "Bearer "+testToken || hits[0].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
}

func TestConfigPutFullBody(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/configuration" {
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","version":"v1","values":{"name":"Beta"},"editable":true,"updatedAt":"2026-01-02T04:00:00Z","secrets":{"version":"s1","configured":true,"pending":false}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	body := `{"expectedSetupID":"setup-a","expectedUpdatedAt":"2026-01-02T03:04:05Z","version":"v1","values":{"name":"Beta"}}`
	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	opts.Stdin = strings.NewReader(body)
	if code := Run([]string{"config", "put", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"name": "Beta"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Method != http.MethodPut || hits[0].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
	var put map[string]json.RawMessage
	if err := json.Unmarshal([]byte(hits[0].Body), &put); err != nil {
		t.Fatal(err)
	}
	if string(put["expectedSetupID"]) != `"setup-a"` || string(put["version"]) != `"v1"` {
		t.Fatalf("body %s", hits[0].Body)
	}
}

func TestConfigPutAutofillFromGET(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/configuration":
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","version":"v1","values":{"name":"Alpha"},"editable":true,"updatedAt":"2026-01-02T03:04:05Z","secrets":{"version":"s1","configured":true,"pending":false}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/configuration":
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","version":"v1","values":{"name":"Beta"},"editable":true,"updatedAt":"2026-01-02T04:00:00Z","secrets":{"version":"s1","configured":true,"pending":false}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	opts.Stdin = strings.NewReader(`{"values":{"name":"Beta"}}`)
	if code := Run([]string{"config", "put", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"name": "Beta"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 2 {
		t.Fatalf("hits %+v", hits)
	}
	if hits[0].Method != http.MethodGet || hits[0].Path != "/api/v1/servers/srv-1/configuration" {
		t.Fatalf("first hit %+v", hits[0])
	}
	put := hits[1]
	if put.Method != http.MethodPut || put.Key != "" {
		t.Fatalf("put %+v", put)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(put.Body), &body); err != nil {
		t.Fatal(err)
	}
	if string(body["expectedSetupID"]) != `"setup-a"` {
		t.Fatalf("expectedSetupID %s", body["expectedSetupID"])
	}
	if string(body["version"]) != `"v1"` {
		t.Fatalf("version %s", body["version"])
	}
	if !strings.Contains(string(body["expectedUpdatedAt"]), "2026-01-02T03:04:05") {
		t.Fatalf("expectedUpdatedAt %s", body["expectedUpdatedAt"])
	}
}

func TestConfigPutFile(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","version":"v1","values":{"name":"FromFile"},"editable":true,"updatedAt":"2026-01-02T04:00:00Z","secrets":{"version":"s1","configured":false,"pending":false}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "put.json")
	if err := os.WriteFile(path, []byte(`{"expectedSetupID":"setup-a","expectedUpdatedAt":"2026-01-02T03:04:05Z","version":"v1","values":{"name":"FromFile"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"config", "put", "srv-1", "--file", path}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"FromFile"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Method != http.MethodPut {
		t.Fatalf("hits %+v", hits)
	}
}

func TestConfigPutMissingValuesFailsBeforeAPI(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	opts.Stdin = strings.NewReader(`{"expectedSetupID":"setup-a","version":"v1"}`)
	if code := Run([]string{"config", "put", "srv-1"}, opts); code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "values") {
		t.Fatalf("stderr %q", stderr.String())
	}
	if hits := rec.snapshot(); len(hits) != 0 {
		t.Fatalf("unexpected hits %+v", hits)
	}
}

func TestConfigPutInvalidJSONFailsBeforeAPI(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	opts.Stdin = strings.NewReader(`[1,2,3]`)
	if code := Run([]string{"config", "put", "srv-1"}, opts); code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "JSON object") {
		t.Fatalf("stderr %q", stderr.String())
	}
	if hits := rec.snapshot(); len(hits) != 0 {
		t.Fatalf("unexpected hits %+v", hits)
	}
}

func TestConfigPutRunningSurfacesAPIError(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/configuration":
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","version":"v1","values":{"name":"Alpha"},"editable":false,"updatedAt":"2026-01-02T03:04:05Z","secrets":{"version":"s1","configured":true,"pending":false}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/configuration":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"the Server must be confirmed stopped before changing configuration"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	opts.Stdin = strings.NewReader(`{"values":{"name":"Beta"}}`)
	if code := Run([]string{"config", "put", "srv-1"}, opts); code == 0 {
		t.Fatal("expected failure while Running")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "server_not_confirmed_stopped") {
		t.Fatalf("stderr %q", stderr.String())
	}
	hits := rec.snapshot()
	var putCount int
	for _, item := range hits {
		if item.Method == http.MethodPut {
			putCount++
		}
	}
	if putCount != 1 {
		t.Fatalf("expected one PUT, hits %+v", hits)
	}
}

func TestSchema(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/games/factorio/management-schema" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"gameID":"factorio","configuration":{"sections":[{"id":"broadcast"}]}}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"schema", "factorio"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"gameID": "factorio"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Key != "" || hits[0].Auth != "Bearer "+testToken {
		t.Fatalf("hits %+v", hits)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"management-schema", "factorio"}, opts); code != 0 {
		t.Fatalf("alias exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"broadcast"`) {
		t.Fatalf("alias stdout %q", stdout.String())
	}
}

func TestModsList(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/servers/srv-1/mods" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":false}}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "list", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"setupID": "setup-a"`) || strings.Contains(out, `"mods"`) {
		t.Fatalf("stdout %q", out)
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Auth != "Bearer "+testToken || hits[0].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
}

func TestModsSearchAndShow(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods/catalog":
			if r.URL.Query().Get("q") != "tiny" || r.URL.Query().Get("category") != "logistics" {
				t.Fatalf("query %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"catalog":{"query":"tiny","results":[{"providerModID":"tiny-mod","title":"Tiny Mod"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods/catalog/tiny-mod":
			_, _ = io.WriteString(w, `{"mod":{"providerModID":"tiny-mod","title":"Tiny Mod","latestVersion":"1.2.3"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "search", "srv-1", "--query", "tiny", "--category", "logistics"}, opts); code != 0 {
		t.Fatalf("search exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"providerModID": "tiny-mod"`) || strings.Contains(stdout.String(), `"catalog"`) {
		t.Fatalf("search stdout %q", stdout.String())
	}

	opts, stdout, stderr = testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "show", "srv-1", "tiny-mod"}, opts); code != 0 {
		t.Fatalf("show exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"latestVersion": "1.2.3"`) || strings.Contains(stdout.String(), `"mod"`) {
		t.Fatalf("show stdout %q", stdout.String())
	}
	_ = rec.snapshot()
}

func TestModsStageAutofillExpectedSetup(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":true}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/mods/staged-selection":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":true,"staged":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod","stageID":"stage-sha"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "stage", "srv-1", "tiny-mod", "--provider", "factorio-mod-portal"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"stageID": "stage-sha"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 2 {
		t.Fatalf("hits %+v", hits)
	}
	if hits[0].Method != http.MethodGet || hits[1].Method != http.MethodPut || hits[1].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(hits[1].Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["expectedSetupID"] != "setup-a" || body["providerID"] != "factorio-mod-portal" || body["providerModID"] != "tiny-mod" {
		t.Fatalf("body %#v", body)
	}
}

func TestModsApplyAutofillStageID(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"staged":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod","stageID":"stage-sha"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/apply":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"enabled":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "apply", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"providerModID": "tiny-mod"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 2 || hits[1].Method != http.MethodPost || hits[1].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(hits[1].Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["expectedSetupID"] != "setup-a" || body["stageID"] != "stage-sha" {
		t.Fatalf("body %#v", body)
	}
}

func TestModsApplyNothingStagedFailsClearly(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods" {
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "apply", "srv-1"}, opts); code == 0 {
		t.Fatal("expected failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no staged mod to apply") {
		t.Fatalf("stderr %q", stderr.String())
	}
	hits := rec.snapshot()
	for _, item := range hits {
		if item.Method == http.MethodPost {
			t.Fatalf("unexpected POST %+v", hits)
		}
	}
}

func TestModsDiscardAndCredentials(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":false}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/mods/credentials":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":true}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/discard":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "credentials", "srv-1", "--username", "player", "--token", "secret"}, opts); code != 0 {
		t.Fatalf("credentials exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"credentialsConfigured": true`) {
		t.Fatalf("credentials stdout %q", stdout.String())
	}

	opts, stdout, stderr = testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "discard", "srv-1", "--expected-setup", "setup-a"}, opts); code != 0 {
		t.Fatalf("discard exit %d stderr %s", code, stderr.String())
	}
	hits := rec.snapshot()
	var discard hit
	for _, item := range hits {
		if item.Method == http.MethodPost && item.Path == "/api/v1/servers/srv-1/mods/discard" {
			discard = item
		}
	}
	if discard.Method == "" || discard.Key != "" || !strings.Contains(discard.Body, `"expectedSetupID":"setup-a"`) {
		t.Fatalf("discard hit %+v from %+v", discard, hits)
	}
}

func TestModsStageRequiresProvider(t *testing.T) {
	opts, _, stderr := testOptions(t.TempDir(), "https://genosservers.com", testToken, nil)
	if code := Run([]string{"mods", "stage", "srv-1", "tiny-mod"}, opts); code != 2 {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--provider") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestModsApplyRunningSurfacesAPIError(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":false,"staged":{"stageID":"stage-sha","providerModID":"tiny-mod"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/apply":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"mod changes can be applied only while the Server is confirmed stopped"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"mods", "apply", "srv-1"}, opts); code == 0 {
		t.Fatal("expected failure while Running")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "server_not_confirmed_stopped") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestSavesExportWaitAndOutput(t *testing.T) {
	rec := &recorder{}
	var gets int
	var downloadHits int
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-exports":
			if r.Header.Get("Idempotency-Key") != "" {
				t.Fatalf("unexpected idempotency key")
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"export":{"id":"exp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"pending","progressPercent":0,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/save-exports/exp-1":
			gets++
			if gets == 1 {
				_, _ = io.WriteString(w, `{"export":{"id":"exp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"running","progressPercent":40,"stage":"pack","requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"export":{"id":"exp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"succeeded","progressPercent":100,"downloadURL":"`+"http://"+r.Host+`/file.zip","archiveName":"save.zip","archiveBytes":9,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/file.zip":
			downloadHits++
			w.Header().Set("Content-Type", "application/zip")
			_, _ = io.WriteString(w, "SAVEBYTES")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "save.zip")
	opts, stdout, stderr := testOptions(dir, server.URL, testToken, nil)
	if code := Run([]string{"saves", "export", "srv-1", "--wait", "--interval", "100ms", "--timeout", "1m", "--output", outPath}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "succeeded"`) || !strings.Contains(stdout.String(), `"id": "exp-1"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "downloadURL") || !strings.Contains(stderr.String(), "running") {
		t.Fatalf("stderr %q", stderr.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil || string(data) != "SAVEBYTES" || downloadHits != 1 {
		t.Fatalf("file %q err=%v hits=%d", data, err, downloadHits)
	}
	hits := rec.snapshot()
	if len(hits) < 2 || hits[0].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
}

func TestSavesExportFailedExitsNonZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"export":{"id":"exp-9","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"pending","progressPercent":0,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		default:
			_, _ = io.WriteString(w, `{"export":{"id":"exp-9","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"failed","progressPercent":10,"message":"boom","requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"saves", "export", "srv-1", "--wait", "--interval", "100ms", "--timeout", "1m"}, opts); code == 0 {
		t.Fatalf("expected non-zero, stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "failed"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestSavesImportWaitRequiresYes(t *testing.T) {
	var uploadAuth, createKey string
	var createBody string
	var uploadServer *httptest.Server
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports":
			createBody = string(payload)
			createKey = r.Header.Get("Idempotency-Key")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"import":{"id":"imp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"awaiting_upload","progressPercent":0,"uploadURL":"`+uploadServer.URL+`/put","fileName":"world.zip","mediaType":"application/zip","archiveBytes":4,"addonSetDiffers":false,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports/imp-1/validate":
			_, _ = io.WriteString(w, `{"import":{"id":"imp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"ready","progressPercent":40,"addonSetDiffers":false,"review":{"fileName":"world.zip","archiveBytes":4,"irreversible":true},"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	uploadServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadAuth = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != "application/zip" {
			t.Fatalf("content-type %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "zip!" {
			t.Fatalf("body %q", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadServer.Close()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "world.zip")
	if err := os.WriteFile(filePath, []byte("zip!"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, stdout, stderr := testOptions(dir, api.URL, testToken, nil)
	code := Run([]string{"saves", "import", "srv-1", "--file", filePath, "--wait", "--interval", "100ms", "--timeout", "1m"}, opts)
	if code == 0 {
		t.Fatalf("expected non-zero without --yes")
	}
	if !strings.Contains(createBody, `"fileName":"world.zip"`) || createKey != "" {
		t.Fatalf("create body=%s key=%q", createBody, createKey)
	}
	if !strings.Contains(stdout.String(), `"status": "ready"`) || !strings.Contains(stderr.String(), "import-replace") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if uploadAuth != "" {
		t.Fatalf("upload sent auth %q", uploadAuth)
	}
}

func TestSavesImportWaitYesReplace(t *testing.T) {
	var replaceBody string
	var gets int
	var uploadServer *httptest.Server
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"import":{"id":"imp-2","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"awaiting_upload","progressPercent":0,"uploadURL":"`+uploadServer.URL+`/put","fileName":"world.zip","mediaType":"application/zip","archiveBytes":4,"addonSetDiffers":true,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports/imp-2/validate":
			_, _ = io.WriteString(w, `{"import":{"id":"imp-2","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"ready","progressPercent":40,"addonSetDiffers":true,"review":{"addonSetDiffers":true},"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports/imp-2/replace":
			replaceBody = string(payload)
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"import":{"id":"imp-2","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"pending","progressPercent":50,"addonSetDiffers":true,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/save-imports/imp-2":
			gets++
			if gets == 1 {
				_, _ = io.WriteString(w, `{"import":{"id":"imp-2","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"running","progressPercent":80,"recovery":"snapshot-created","addonSetDiffers":true,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"import":{"id":"imp-2","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"succeeded","progressPercent":100,"recovery":"snapshot-created","addonSetDiffers":true,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	uploadServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadServer.Close()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "world.zip")
	if err := os.WriteFile(filePath, []byte("zip!"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, stdout, stderr := testOptions(dir, api.URL, testToken, nil)
	code := Run([]string{"saves", "import", "srv-1", "--file", filePath, "--wait", "--yes", "--apply-save-mods", "--interval", "100ms", "--timeout", "1m"}, opts)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "succeeded"`) || !strings.Contains(stdout.String(), `"recovery": "snapshot-created"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(replaceBody, `"acknowledged":true`) || !strings.Contains(replaceBody, `"applySaveMods":true`) {
		t.Fatalf("replace body %s", replaceBody)
	}
	if !strings.Contains(stderr.String(), "recovery=snapshot-created") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestSavesImportReplaceRequiresYes(t *testing.T) {
	opts, _, stderr := testOptions(t.TempDir(), "http://example.invalid", testToken, nil)
	if code := Run([]string{"saves", "import-replace", "srv-1", "imp-1"}, opts); code != 2 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestSetupsListPrintsCreatableGames(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/servers/srv-1/setups" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a","capacity":{"used":1,"limit":5},"creatableGames":[{"id":"factorio","name":"Factorio"},{"id":"rust","name":"Rust"}]}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"setups", "srv-1"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "setup-a\tMain\tFactorio\t*") {
		t.Fatalf("stdout missing setup line: %q", got)
	}
	if !strings.Contains(got, "creatable\tfactorio\tFactorio\n") || !strings.Contains(got, "creatable\trust\tRust\n") {
		t.Fatalf("stdout missing creatable lines: %q", got)
	}
}

func TestRenameServerHappyPath(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/servers/srv-1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Renamed","game":{"name":"Factorio"},"status":"Stopped"}}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"rename", "srv-1", "Renamed"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"name": "Renamed"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Method != http.MethodPatch || hits[0].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
	if !strings.Contains(hits[0].Body, `"name":"Renamed"`) {
		t.Fatalf("body %s", hits[0].Body)
	}
}

func TestRenameServerSurfacesNotStopped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"the Server must be confirmed stopped before changing its name"}}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"rename", "srv-1", "Nope"}, opts); code == 0 {
		t.Fatal("expected failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "server_not_confirmed_stopped") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestCreateSetup(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/servers/srv-1/setups" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"setup":{"id":"setup-new","name":"Factorio","game":{"name":"Factorio"}},"capacity":{"used":2,"limit":5},"server":{"id":"srv-1","name":"Alpha","status":"Stopped"}}`)
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"create-setup", "srv-1", "--game", "factorio"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "setup-new"`) || !strings.Contains(stdout.String(), `"used": 2`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Key != "" || !strings.Contains(hits[0].Body, `"gameID":"factorio"`) {
		t.Fatalf("hits %+v", hits)
	}
}

func TestRenameSetupAutofillsExpected(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setups":
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Factorio","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a"}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/servers/srv-1/setups/setup-a":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Alpha","status":"Stopped"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"rename-setup", "srv-1", "setup-a", "Main factory"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "srv-1"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 2 || hits[0].Method != http.MethodGet || hits[1].Method != http.MethodPatch || hits[1].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
	body := hits[1].Body
	if !strings.Contains(body, `"name":"Main factory"`) || !strings.Contains(body, `"expectedName":"Factorio"`) || !strings.Contains(body, `"expectedSelectedSetupID":"setup-a"`) {
		t.Fatalf("body %s", body)
	}
}

func TestDeleteSetupRequiresYes(t *testing.T) {
	opts, _, stderr := testOptions(t.TempDir(), "http://example.invalid", testToken, nil)
	if code := Run([]string{"delete-setup", "srv-1", "setup-a"}, opts); code != 2 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestDeleteSetupWithYesAutofill(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setups":
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main factory","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/servers/srv-1/setups/setup-a":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Alpha","status":"Stopped"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"delete-setup", "srv-1", "setup-a", "--yes"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id": "srv-1"`) {
		t.Fatalf("stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 2 || hits[1].Method != http.MethodDelete || hits[1].Key != "" {
		t.Fatalf("hits %+v", hits)
	}
	if !strings.Contains(hits[1].Body, `"expectedName":"Main factory"`) || !strings.Contains(hits[1].Body, `"expectedSelectedSetupID":"setup-a"`) {
		t.Fatalf("body %s", hits[1].Body)
	}
}

func TestBroadcastStatusAndSend(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/broadcast":
			_, _ = io.WriteString(w, `{"supported":true,"availability":"available","message":"hello","preview":"hello"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/broadcast":
			_, _ = io.WriteString(w, `{"supported":true,"availability":"available","delivered":true,"message":"hello"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"broadcast-status", "srv-1", "--message", "hello"}, opts); code != 0 {
		t.Fatalf("status exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"availability": "available"`) {
		t.Fatalf("status stdout %q", stdout.String())
	}
	hits := rec.snapshot()
	if len(hits) != 1 || hits[0].Method != http.MethodGet || hits[0].Key != "" {
		t.Fatalf("status hits %+v", hits)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"broadcast", "srv-1", "--message", "hello"}, opts); code != 0 {
		t.Fatalf("broadcast exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"delivered": true`) {
		t.Fatalf("broadcast stdout %q", stdout.String())
	}
	hits = rec.snapshot()
	if len(hits) != 2 || hits[1].Method != http.MethodPost || hits[1].Key != "" || !strings.Contains(hits[1].Body, `"message":"hello"`) {
		t.Fatalf("broadcast hits %+v", hits)
	}
}

func TestBroadcastSurfacesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"broadcast_rate_limited","message":"Wait a moment before sending another notice."}}`)
	}))
	defer server.Close()

	opts, _, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"broadcast", "srv-1", "--message", "hi"}, opts); code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String(), "broadcast_rate_limited") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestMeDashboardCatalog(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/me":
			_, _ = io.WriteString(w, `{"id":"user-1","clerkUserID":"clerk_1","sessionID":"sess","primaryEmail":"a@example.com","isPlatformAdmin":false}`)
		case "/api/v1/dashboard":
			_, _ = io.WriteString(w, `{"servers":[],"profiles":[{"id":"p1","name":"Alpha","game":{"name":"Factorio"},"serverName":"srv","selected":true}],"profileCapacity":{"used":1,"limit":5}}`)
		case "/api/v1/catalog":
			_, _ = io.WriteString(w, `{"games":[{"id":"factorio","name":"Factorio"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"me"}, opts); code != 0 {
		t.Fatalf("me exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"clerkUserID": "clerk_1"`) {
		t.Fatalf("me stdout %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"dashboard"}, opts); code != 0 {
		t.Fatalf("dashboard exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"profileCapacity"`) {
		t.Fatalf("dashboard stdout %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"catalog"}, opts); code != 0 {
		t.Fatalf("catalog exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"factorio"`) {
		t.Fatalf("catalog stdout %q", stdout.String())
	}
}

func TestProfilesCreateRenameDeleteAutofill(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/dashboard":
			_, _ = io.WriteString(w, `{"servers":[],"profiles":[{"id":"prof-1","name":"Old","game":{"name":"Factorio"},"selected":false}],"profileCapacity":{"used":1,"limit":5}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"setup":{"id":"prof-new","name":"Factorio","game":{"name":"Factorio"}},"capacity":{"used":2,"limit":5}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/profiles/prof-1":
			_, _ = io.WriteString(w, `{"setup":{"id":"prof-1","name":"New","game":{"name":"Factorio"}}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/profiles/prof-1":
			_, _ = io.WriteString(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profiles/games":
			_, _ = io.WriteString(w, `{"setups":[],"capacity":{"used":1,"limit":5},"creatableGames":[{"id":"factorio","name":"Factorio"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"profiles"}, opts); code != 0 {
		t.Fatalf("profiles exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "prof-1\tOld\tFactorio") || !strings.Contains(stdout.String(), "capacity\t1\t5") {
		t.Fatalf("profiles list %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"profiles", "games"}, opts); code != 0 {
		t.Fatalf("games exit %d stderr %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"profiles", "create", "--game", "factorio"}, opts); code != 0 {
		t.Fatalf("create exit %d stderr %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"profiles", "rename", "prof-1", "New"}, opts); code != 0 {
		t.Fatalf("rename exit %d stderr %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"profiles", "delete", "prof-1"}, opts); code != 2 {
		t.Fatalf("delete without --yes exit %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"profiles", "delete", "prof-1", "--yes"}, opts); code != 0 {
		t.Fatalf("delete exit %d stderr %s", code, stderr.String())
	}
	hits := rec.snapshot()
	var renameBody, deleteBody string
	for _, hit := range hits {
		if hit.Method == http.MethodPatch {
			renameBody = hit.Body
		}
		if hit.Method == http.MethodDelete {
			deleteBody = hit.Body
		}
	}
	if !strings.Contains(renameBody, `"expectedName":"Old"`) || !strings.Contains(renameBody, `"name":"New"`) {
		t.Fatalf("rename body %s", renameBody)
	}
	if !strings.Contains(deleteBody, `"expectedName":"Old"`) {
		t.Fatalf("delete body %s", deleteBody)
	}
}

func TestProfilesConfigGetPutAutofill(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profiles/prof-1/configuration":
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"prof-1","gameID":"factorio","version":"v3","values":{"x":1},"editable":true,"updatedAt":"2026-01-02T03:04:05Z","secrets":{"version":"s1","configured":false,"pending":false}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/profiles/prof-1/configuration":
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"prof-1","gameID":"factorio","version":"v4","values":{"x":2},"editable":true,"updatedAt":"2026-01-02T04:00:00Z","secrets":{"version":"s1","configured":false,"pending":false}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"profiles", "config", "get", "prof-1"}, opts); code != 0 {
		t.Fatalf("get exit %d stderr %s", code, stderr.String())
	}
	opts.Stdin = strings.NewReader(`{"values":{"x":2}}`)
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"profiles", "config", "put", "prof-1"}, opts); code != 0 {
		t.Fatalf("put exit %d stderr %s", code, stderr.String())
	}
	var put hit
	for _, h := range rec.snapshot() {
		if h.Method == http.MethodPut {
			put = h
		}
	}
	if !strings.Contains(put.Body, `"expectedSetupID":"prof-1"`) || !strings.Contains(put.Body, `"version":"v3"`) {
		t.Fatalf("put body %s", put.Body)
	}
}

func TestSetupCopyStartWaitSuccessAndFail(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rec := &recorder{}
		polls := 0
		server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setup-copy-destinations":
				_, _ = io.WriteString(w, `{"destinations":[{"serverID":"srv-2","name":"Beta","eligible":true}]}`)
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/setups/setup-a/copies":
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, `{"copy":{"id":"copy-1","status":"pending","progressPercent":0,"mode":"copy"}}`)
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setup-copies/copy-1":
				polls++
				if polls < 2 {
					_, _ = io.WriteString(w, `{"copy":{"id":"copy-1","status":"running","progressPercent":40,"mode":"copy"}}`)
					return
				}
				_, _ = io.WriteString(w, `{"copy":{"id":"copy-1","status":"succeeded","progressPercent":100,"mode":"copy"}}`)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
		if code := Run([]string{"setup-copy", "destinations", "srv-1"}, opts); code != 0 {
			t.Fatalf("destinations exit %d stderr %s", code, stderr.String())
		}
		stdout.Reset()
		stderr.Reset()
		if code := Run([]string{"setup-copy", "start", "srv-1", "setup-a", "--destination", "srv-2", "--mode", "copy", "--wait", "--interval", "100ms", "--timeout", "5s"}, opts); code != 0 {
			t.Fatalf("start wait exit %d stderr %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), `"status": "succeeded"`) {
			t.Fatalf("stdout %q", stdout.String())
		}
	})
	t.Run("fail", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost:
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, `{"copy":{"id":"copy-2","status":"pending","progressPercent":0}}`)
			default:
				_, _ = io.WriteString(w, `{"copy":{"id":"copy-2","status":"failed","progressPercent":10,"message":"boom"}}`)
			}
		}))
		defer server.Close()
		opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
		if code := Run([]string{"setup-copy", "start", "srv-1", "setup-a", "--destination", "srv-2", "--mode", "transfer", "--wait", "--interval", "100ms", "--timeout", "5s"}, opts); code == 0 {
			t.Fatal("expected failure")
		}
		if !strings.Contains(stdout.String(), `"status": "failed"`) {
			t.Fatalf("stdout %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "setup copy failed") {
			t.Fatalf("stderr %q", stderr.String())
		}
	})
}

func TestAuthTokensCreateRevoke(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(rec.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/tokens":
			_, _ = io.WriteString(w, `{"tokens":[{"id":"tok-1","tokenPrefix":"genos_abc","name":"laptop"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/tokens":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":{"id":"tok-2","tokenPrefix":"genos_xyz","name":"ci"},"secret":"genos_xyz_PLAINTEXT_ONCE"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/auth/tokens/tok-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"auth", "tokens"}, opts); code != 0 {
		t.Fatalf("tokens exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"tokenPrefix": "genos_abc"`) {
		t.Fatalf("tokens stdout %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"auth", "token-create", "--name", "ci", "--client-name", "genos-cli", "--machine-name", "box"}, opts); code != 0 {
		t.Fatalf("token-create exit %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "genos_xyz_PLAINTEXT_ONCE") {
		t.Fatalf("secret missing from stdout %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "genos_xyz_PLAINTEXT_ONCE") {
		t.Fatalf("secret leaked to stderr %q", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"auth", "token-revoke", "tok-1"}, opts); code != 2 {
		t.Fatalf("revoke without --yes exit %d", code)
	}
	if code := Run([]string{"auth", "token-revoke", "tok-1", "--yes"}, opts); code != 0 {
		t.Fatalf("revoke exit %d stderr %s", code, stderr.String())
	}
	hits := rec.snapshot()
	var createBody string
	for _, hit := range hits {
		if hit.Method == http.MethodPost {
			createBody = hit.Body
		}
	}
	if !strings.Contains(createBody, `"name":"ci"`) || !strings.Contains(createBody, `"clientName":"genos-cli"`) || !strings.Contains(createBody, `"machineName":"box"`) {
		t.Fatalf("create body %s", createBody)
	}
}
