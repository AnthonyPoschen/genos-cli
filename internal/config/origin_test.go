package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOrigin(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr string
	}{
		{in: "https://genosservers.com", want: "https://genosservers.com"},
		{in: "https://GenosServers.com/", want: "https://genosservers.com"},
		{in: "https://genosservers.com:443", want: "https://genosservers.com"},
		{in: "http://localhost", want: "http://localhost"},
		{in: "http://LocalHost:8000", want: "http://localhost:8000"},
		{in: "http://127.0.0.1:8000", want: "http://127.0.0.1:8000"},
		{in: "http://genos.localhost:8000", want: "http://genos.localhost:8000"},
		{in: "HTTP://GENOS.LOCALHOST", want: "http://genos.localhost"},
		{in: "http://localhost:80", want: "http://localhost"},
		{in: "http://example.com", wantErr: "https"},
		{in: "http://127.0.0.2", wantErr: "https"},
		{in: "http://localhost.example.com", wantErr: "https"},
		{in: "http://[::1]", wantErr: "https"},
		{in: "ftp://genosservers.com", wantErr: "not supported"},
		{in: "genosservers.com", wantErr: "scheme"},
		{in: "https://user:pass@genosservers.com", wantErr: "user info"},
		{in: "https://genosservers.com/api", wantErr: "path"},
		{in: "https://genosservers.com?q=1", wantErr: "query"},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			got, err := ValidateOrigin(test.in)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ValidateOrigin(%q) = %q, %v; want error containing %q", test.in, got, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateOrigin(%q): %v", test.in, err)
			}
			if got != test.want {
				t.Fatalf("ValidateOrigin(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestResolveOrigin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "# comment\ncurrentHost = \"https://genosservers.com\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveOrigin("", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://genosservers.com" {
		t.Fatalf("config origin = %q", got)
	}
	got, err = ResolveOrigin("http://127.0.0.1:8000", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:8000" {
		t.Fatalf("env origin = %q", got)
	}
	if _, err := ResolveOrigin("http://example.com", path); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("public http error = %v", err)
	}

	host, ok, err := ParseCurrentHost("currentHost = 'http://genos.localhost:8000'\n")
	if err != nil || !ok || host != "http://genos.localhost:8000" {
		t.Fatalf("single quote parse = %q, %v, %v", host, ok, err)
	}
}
