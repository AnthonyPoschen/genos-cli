//go:build e2e

package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const e2eToken = "abcdefghijklmnopqrstuvwxyz"

func TestBinaryEndToEnd(t *testing.T) {
	bin := buildGenos(t)
	home := t.TempDir()
	xdg := t.TempDir()

	var mu sync.Mutex
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/me" && r.URL.Path != "/api/v1/catalog" && r.URL.Path != "/api/v1/auth/tokens" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+e2eToken {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"unauthorized","message":"token rejected"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/me":
			_, _ = io.WriteString(w, `{"id":"user-1","clerkUserID":"clerk_1","sessionID":"sess","primaryEmail":"a@example.com","isPlatformAdmin":false}`)
		case "/api/v1/catalog":
			_, _ = io.WriteString(w, `{"games":[{"id":"factorio","name":"Factorio"}]}`)
		case "/api/v1/auth/tokens":
			_, _ = io.WriteString(w, `{"tokens":[{"id":"tok-1","tokenPrefix":"genos_abc","name":"laptop","clientName":"genos","machineName":"box","createdAt":"2026-01-02T03:04:05Z","lastUsedAt":"2026-01-03T03:04:05Z"}]}`)
		}
	}))
	t.Cleanup(srv.Close)

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + xdg,
		"GENOS_HOST=" + srv.URL,
		"GENOS_TOKEN=" + e2eToken,
		"TERM=dumb",
	}

	tokensOut, tokensErr, tokensCode := runGenos(t, bin, env, "auth", "tokens")
	if tokensCode != 0 {
		t.Fatalf("auth tokens exit %d stderr %s", tokensCode, tokensErr)
	}
	if !strings.Contains(tokensOut, "ID     PREFIX     NAME    CLIENT  MACHINE  CREATED     LAST USED   EXPIRES  REVOKED") {
		t.Fatalf("auth tokens table header %q", tokensOut)
	}
	if !strings.Contains(tokensOut, "tok-1  genos_abc  laptop  genos   box      2026-01-02  2026-01-03  never") {
		t.Fatalf("auth tokens table row %q", tokensOut)
	}
	if strings.Contains(tokensOut, `"tokenPrefix"`) {
		t.Fatalf("auth tokens default is JSON %q", tokensOut)
	}

	tokensJSON, tokensJSONErr, tokensJSONCode := runGenos(t, bin, env, "auth", "tokens", "--json")
	if tokensJSONCode != 0 {
		t.Fatalf("auth tokens --json exit %d stderr %s", tokensJSONCode, tokensJSONErr)
	}
	var listed struct {
		Tokens []struct {
			ID          string `json:"id"`
			TokenPrefix string `json:"tokenPrefix"`
			Name        string `json:"name"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(tokensJSON), &listed); err != nil {
		t.Fatalf("auth tokens --json %q: %v", tokensJSON, err)
	}
	if len(listed.Tokens) != 1 || listed.Tokens[0].ID != "tok-1" || listed.Tokens[0].TokenPrefix != "genos_abc" || listed.Tokens[0].Name != "laptop" {
		t.Fatalf("auth tokens --json payload %+v", listed)
	}

	meOut, meErr, meCode := runGenos(t, bin, env, "me")
	if meCode != 0 {
		t.Fatalf("me exit %d stderr %s", meCode, meErr)
	}
	if !strings.Contains(meOut, "ID      CLERK USER  SESSION  EMAIL          ADMIN") {
		t.Fatalf("me table header %q", meOut)
	}
	if !strings.Contains(meOut, "user-1  clerk_1     sess     a@example.com  false") {
		t.Fatalf("me table row %q", meOut)
	}

	meJSON, meJSONErr, meJSONCode := runGenos(t, bin, env, "me", "--json")
	if meJSONCode != 0 {
		t.Fatalf("me --json exit %d stderr %s", meJSONCode, meJSONErr)
	}
	var me struct {
		ID           string `json:"id"`
		ClerkUserID  string `json:"clerkUserID"`
		PrimaryEmail string `json:"primaryEmail"`
	}
	if err := json.Unmarshal([]byte(meJSON), &me); err != nil {
		t.Fatalf("me --json %q: %v", meJSON, err)
	}
	if me.ID != "user-1" || me.ClerkUserID != "clerk_1" || me.PrimaryEmail != "a@example.com" {
		t.Fatalf("me --json payload %+v", me)
	}

	catalogOut, catalogErr, catalogCode := runGenos(t, bin, env, "catalog")
	if catalogCode != 0 {
		t.Fatalf("catalog exit %d stderr %s", catalogCode, catalogErr)
	}
	if !strings.Contains(catalogOut, "ID        NAME") || !strings.Contains(catalogOut, "factorio  Factorio") {
		t.Fatalf("catalog table %q", catalogOut)
	}

	catalogJSON, catalogJSONErr, catalogJSONCode := runGenos(t, bin, env, "catalog", "--json")
	if catalogJSONCode != 0 {
		t.Fatalf("catalog --json exit %d stderr %s", catalogJSONCode, catalogJSONErr)
	}
	var catalog struct {
		Games []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"games"`
	}
	if err := json.Unmarshal([]byte(catalogJSON), &catalog); err != nil {
		t.Fatalf("catalog --json %q: %v", catalogJSON, err)
	}
	if len(catalog.Games) != 1 || catalog.Games[0].ID != "factorio" || catalog.Games[0].Name != "Factorio" {
		t.Fatalf("catalog --json payload %+v", catalog)
	}

	mu.Lock()
	gotAuths := append([]string(nil), auths...)
	mu.Unlock()
	if len(gotAuths) != 6 {
		t.Fatalf("api calls %d, want 6: %v", len(gotAuths), gotAuths)
	}
	for _, auth := range gotAuths {
		if auth != "Bearer "+e2eToken {
			t.Fatalf("authorization %q", auth)
		}
	}

	badEnv := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + xdg,
		"GENOS_HOST=" + srv.URL,
		"GENOS_TOKEN=not-the-token",
		"TERM=dumb",
	}
	badOut, badErr, badCode := runGenos(t, bin, badEnv, "me")
	if badCode != 1 {
		t.Fatalf("unauthorized me exit %d stdout %q stderr %q", badCode, badOut, badErr)
	}
	if !strings.Contains(badErr, "unauthorized: token rejected") {
		t.Fatalf("unauthorized stderr %q", badErr)
	}
	if badOut != "" {
		t.Fatalf("unauthorized stdout %q", badOut)
	}
}

func buildGenos(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "genos")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/genos")
	cmd.Dir = moduleRoot(t)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./cmd/genos: %v\n%s", err, out)
	}
	return bin
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func runGenos(t *testing.T, bin string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %s: %v", args, err)
		}
		code = exit.ExitCode()
	}
	return outBuf.String(), errBuf.String(), code
}
