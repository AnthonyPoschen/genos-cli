package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func runJSON(t *testing.T, host string, args ...string) map[string]any {
	t.Helper()
	opts, stdout, stderr := testOptions(t.TempDir(), host, testToken, nil)
	if code := Run(args, opts); code != 0 {
		t.Fatalf("%v exit %d stderr %s", args, code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("%v stderr %q", args, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("%v stdout is not one JSON object: %v\n%s", args, err, stdout.String())
	}
	if strings.Contains(stdout.String(), testToken) {
		t.Fatalf("%v stdout leaks the token", args)
	}
	return out
}

func TestReadCommandsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/servers":
			_, _ = io.WriteString(w, `{"servers":[{"id":"srv-1","name":"Alpha","game":{"name":"Factorio"},"status":"Running"}]}`)
		case "/api/v1/servers/srv-1":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Alpha","game":{"name":"Factorio"},"status":"Running"}}`)
		case "/api/v1/servers/srv-1/setups":
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a","capacity":{"used":1,"limit":5},"creatableGames":[]}`)
		case "/api/v1/dashboard":
			_, _ = io.WriteString(w, `{"profiles":[{"id":"p-1","name":"Main","game":{"name":"Factorio"},"serverName":"Alpha","selected":true}],"profileCapacity":{"used":1,"limit":2}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	list := runJSON(t, server.URL, "server", "list", "--json")
	servers, ok := list["servers"].([]any)
	if !ok || len(servers) != 1 || servers[0].(map[string]any)["id"] != "srv-1" {
		t.Fatalf("server list json %v", list)
	}
	status := runJSON(t, server.URL, "server", "status", "srv-1", "--json")
	if status["id"] != "srv-1" || status["status"] != "Running" {
		t.Fatalf("server status json %v", status)
	}
	chooser := runJSON(t, server.URL, "server", "profile", "list", "srv-1", "--json")
	if chooser["selectedSetupID"] != "setup-a" || len(chooser["setups"].([]any)) != 1 {
		t.Fatalf("server profile list json %v", chooser)
	}
	profiles := runJSON(t, server.URL, "profile", "list", "--json")
	if len(profiles["profiles"].([]any)) != 1 || profiles["profileCapacity"].(map[string]any)["limit"] != float64(2) {
		t.Fatalf("profile list json %v", profiles)
	}
	auth := runJSON(t, server.URL, "auth", "status", "--json")
	if auth["origin"] != server.URL || auth["source"] == "" || auth["prefix"] == "" {
		t.Fatalf("auth status json %v", auth)
	}
}

func TestReadCommandJSONErrorGoesToStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()
	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	code := Run([]string{"server", "status", "srv-9", "--json"}, opts)
	if code != int(ExitNotFound) {
		t.Fatalf("exit %d want %d stderr %s", code, ExitNotFound, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q want empty", stdout.String())
	}
	var report struct {
		Error struct {
			Code     string `json:"code"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &report); err != nil || report.Error.ExitCode != int(ExitNotFound) {
		t.Fatalf("stderr %q err %v", stderr.String(), err)
	}
}

func TestEveryReadCommandAcceptsJSON(t *testing.T) {
	for _, path := range readCommands {
		opts, stdout, stderr := testOptions(t.TempDir(), "", "", nil)
		args := append(strings.Fields(path), "--json", "--help")
		if code := Run(args, opts); code != 0 {
			t.Fatalf("%s rejects --json: exit %d stderr %s", path, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "--json") {
			t.Fatalf("%s help lacks --json", path)
		}
	}
}

// TestCommandClassification walks the real command tree so a new command
// fails CI until it is added to readCommands or writeCommands.
func TestCommandClassification(t *testing.T) {
	r := newRunner(Options{})
	live := map[string]bool{}
	for _, c := range commandCatalog(newRootCmd(r)) {
		live[c.Path] = true
	}
	seen := map[string]string{}
	for list, paths := range map[string][]string{"read": readCommands, "write": writeCommands} {
		for _, path := range paths {
			if prev, ok := seen[path]; ok {
				t.Errorf("%q classified twice (%s, %s)", path, prev, list)
			}
			seen[path] = list
			if !live[path] {
				t.Errorf("%q in %s list but no such runnable command", path, list)
			}
		}
	}
	for path := range live {
		if _, ok := seen[path]; !ok {
			t.Errorf("%q is not classified: add it to readCommands or writeCommands", path)
		}
	}
}

func TestCommandsJSON(t *testing.T) {
	opts, stdout, stderr := testOptions(t.TempDir(), "", "", nil)
	if code := Run([]string{"commands", "--json"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	var catalog struct {
		Commands []commandInfo `json:"commands"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	byPath := map[string]commandInfo{}
	for _, c := range catalog.Commands {
		byPath[c.Path] = c
	}
	for _, path := range readCommands {
		if c := byPath[path]; !c.ReadOnly || c.Mutating || !c.JSON {
			t.Fatalf("read command %q entry %+v", path, c)
		}
	}
	for _, path := range writeCommands {
		if c := byPath[path]; c.ReadOnly || !c.Mutating {
			t.Fatalf("write command %q entry %+v", path, c)
		}
	}
	if c := byPath["server stop"]; !c.HasYesFlag || !c.Mutating {
		t.Fatalf("server stop %+v", c)
	}
	if _, ok := byPath["server"]; ok {
		t.Fatal("container commands should not be listed")
	}

	opts, stdout, _ = testOptions(t.TempDir(), "", "", nil)
	if code := Run([]string{"commands"}, opts); code != 0 || !strings.Contains(stdout.String(), "server list") {
		t.Fatalf("text commands exit %d out %q", code, stdout.String())
	}
}
