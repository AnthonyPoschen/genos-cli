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
	configWithHost := filepath.Join(dir, "with-host.toml")
	if err := os.WriteFile(configWithHost, []byte("# comment\ncurrentHost = \"https://staging.example.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configEmpty := filepath.Join(dir, "empty.toml")
	if err := os.WriteFile(configEmpty, []byte("# no currentHost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dir, "missing.toml")

	tests := []struct {
		name       string
		hostEnv    string
		configPath string
		want       string
		wantErr    string
	}{
		{
			name:       "default when no env and missing config",
			hostEnv:    "",
			configPath: missingPath,
			want:       DefaultOrigin,
		},
		{
			name:       "default when no env and empty config",
			hostEnv:    "",
			configPath: configEmpty,
			want:       DefaultOrigin,
		},
		{
			name:       "whitespace env treated as unset falls back to config",
			hostEnv:    "   ",
			configPath: configWithHost,
			want:       "https://staging.example.com",
		},
		{
			name:       "config currentHost wins over default",
			hostEnv:    "",
			configPath: configWithHost,
			want:       "https://staging.example.com",
		},
		{
			name:       "GENOS_HOST wins over config",
			hostEnv:    "http://127.0.0.1:8000",
			configPath: configWithHost,
			want:       "http://127.0.0.1:8000",
		},
		{
			name:       "GENOS_HOST wins over missing config",
			hostEnv:    "http://genos.localhost:8000",
			configPath: missingPath,
			want:       "http://genos.localhost:8000",
		},
		{
			name:       "public http rejected",
			hostEnv:    "http://example.com",
			configPath: configWithHost,
			wantErr:    "https",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveOrigin(test.hostEnv, test.configPath)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ResolveOrigin(%q, %q) = %q, %v; want error containing %q", test.hostEnv, test.configPath, got, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveOrigin(%q, %q): %v", test.hostEnv, test.configPath, err)
			}
			if got != test.want {
				t.Fatalf("ResolveOrigin(%q, %q) = %q, want %q", test.hostEnv, test.configPath, got, test.want)
			}
		})
	}

	host, ok, err := ParseCurrentHost("currentHost = 'http://genos.localhost:8000'\n")
	if err != nil || !ok || host != "http://genos.localhost:8000" {
		t.Fatalf("single quote parse = %q, %v, %v", host, ok, err)
	}
}
