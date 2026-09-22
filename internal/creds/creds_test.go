package creds

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	origin          = "https://genosservers.com"
	processSentinel = "PROCESS-ENV-TOKEN-SHOULD-NOT-LEAK"
	localSecret     = "LOCAL-ENV-SECRET-SHOULD-NOT-BE-READ"
)

type fakeKeyring struct {
	token  string
	getErr error
	setErr error
	origin string
	gets   int
	sets   int
}

func (f *fakeKeyring) Get(gotOrigin string) (string, error) {
	f.gets++
	f.origin = gotOrigin
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.token, nil
}

func (f *fakeKeyring) Set(gotOrigin, secret string) error {
	f.sets++
	f.origin = gotOrigin
	if f.setErr != nil {
		return f.setErr
	}
	f.token = secret
	return nil
}

func TestResolveToken(t *testing.T) {
	t.Setenv("GENOS_TOKEN", processSentinel)
	t.Setenv("GENOS_LOCAL_ENV", processSentinel)

	tests := []struct {
		name      string
		env       string
		ring      *fakeKeyring
		nilRing   bool
		mode      os.FileMode
		noFile    bool
		fileToken string
		fileName  string
		localEnv  string
		wantSrc   string
		wantToken string
		wantErr   string
		wantGets  int
	}{
		{
			name:      "env wins and skips keyring and file",
			env:       "env-token-abcdefghij",
			ring:      &fakeKeyring{token: "keyring-token"},
			mode:      0o644,
			fileToken: "file-token",
			localEnv:  localSecret,
			wantSrc:   SourceEnv,
			wantToken: "env-token-abcdefghij",
			wantGets:  0,
		},
		{
			name:      "keyring is used when env is empty",
			ring:      &fakeKeyring{token: "keyring-token-abcdefghij"},
			mode:      0o644,
			fileToken: "file-token",
			localEnv:  localSecret,
			wantSrc:   SourceKeyring,
			wantToken: "keyring-token-abcdefghij",
			wantGets:  1,
		},
		{
			name:      "keyring error falls through to mode 0600",
			ring:      &fakeKeyring{getErr: errors.New("dbus down")},
			mode:      0o600,
			fileToken: "file-token-abcdefghij",
			localEnv:  localSecret,
			wantSrc:   SourceFile,
			wantToken: "file-token-abcdefghij",
			wantGets:  1,
		},
		{
			name:      "empty keyring secret falls through to file",
			ring:      &fakeKeyring{},
			mode:      0o600,
			fileToken: "file-token-abcdefghij",
			wantSrc:   SourceFile,
			wantToken: "file-token-abcdefghij",
			wantGets:  1,
		},
		{
			name:      "nil keyring falls through to file",
			nilRing:   true,
			mode:      0o600,
			fileToken: "file-token-abcdefghij",
			wantSrc:   SourceFile,
			wantToken: "file-token-abcdefghij",
		},
		{
			name:      "mode 0644 is refused",
			ring:      &fakeKeyring{getErr: errors.New("miss")},
			mode:      0o644,
			fileToken: "file-token-secret",
			wantErr:   "chmod 0600",
			wantGets:  1,
		},
		{
			name:      "mode 0660 is refused",
			ring:      &fakeKeyring{getErr: errors.New("miss")},
			mode:      0o660,
			fileToken: "file-token-secret",
			wantErr:   "chmod 0600",
		},
		{
			name:     "missing file is not found and local.env is not read",
			ring:     &fakeKeyring{getErr: errors.New("miss")},
			noFile:   true,
			localEnv: localSecret,
			wantErr:  "no token",
		},
		{
			name:      "local.env path is refused",
			ring:      &fakeKeyring{getErr: errors.New("miss")},
			mode:      0o600,
			fileToken: localSecret,
			fileName:  "local.env",
			wantErr:   "local.env",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.localEnv != "" {
				writeRaw(t, filepath.Join(dir, "local.env"), test.localEnv, 0o600)
			}
			name := test.fileName
			if name == "" {
				name = "credentials.json"
			}
			path := filepath.Join(dir, name)
			if !test.noFile {
				writeCredentials(t, path, origin, test.fileToken, test.mode)
			}
			var ring Keyring
			if !test.nilRing {
				if test.ring == nil {
					test.ring = &fakeKeyring{getErr: errors.New("miss")}
				}
				ring = test.ring
			}
			got, err := ResolveToken(test.env, ring, File{Path: path, Origin: origin})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, test.wantErr)
				}
				if got.Token != "" {
					t.Fatal("token returned with error")
				}
				if strings.Contains(got.Token, localSecret) || strings.Contains(err.Error(), localSecret) || strings.Contains(err.Error(), processSentinel) {
					t.Fatal("secret leaked into result")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveToken: %v", err)
			}
			if got.Source != test.wantSrc || got.Token != test.wantToken {
				t.Fatalf("got %+v, want source %s token %s", got, test.wantSrc, test.wantToken)
			}
			if got.Token == localSecret || got.Token == processSentinel {
				t.Fatalf("resolved forbidden token %q", got.Token)
			}
			if test.ring != nil && test.ring.gets != test.wantGets {
				t.Fatalf("keyring gets = %d, want %d", test.ring.gets, test.wantGets)
			}
			if test.ring != nil && test.ring.gets > 0 && test.ring.origin != origin {
				t.Fatalf("keyring origin = %q", test.ring.origin)
			}
		})
	}
}

func TestResolveTokenSymlinkToLocalEnv(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "local.env")
	writeRaw(t, target, `{"hosts":{"`+origin+`":{"token":"`+localSecret+`"}}}`+"\n", 0o600)
	link := filepath.Join(dir, "credentials.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveToken("", &fakeKeyring{getErr: errors.New("miss")}, File{Path: link, Origin: origin})
	if err == nil || !strings.Contains(err.Error(), "local.env") {
		t.Fatalf("error = %v", err)
	}
}

func TestKeyringAttributesUseHost(t *testing.T) {
	attributes := KeyringAttributes(origin)
	if attributes["service"] != "genos" || attributes["host"] != origin {
		t.Fatalf("attributes = %#v", attributes)
	}
	if _, ok := attributes["username"]; ok {
		t.Fatal("username attribute must not be used")
	}
}

func TestSave(t *testing.T) {
	t.Run("keyring", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "credentials.json")
		ring := &fakeKeyring{}
		source, err := Save(origin, "saved-token-abcdefghij", ring, path)
		if err != nil {
			t.Fatal(err)
		}
		if source != SourceKeyring || ring.token != "saved-token-abcdefghij" || ring.origin != origin {
			t.Fatalf("source %s ring %+v", source, ring)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file exists after keyring save: %v", err)
		}
	})

	t.Run("file fallback creates directory 0700 and file 0600", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "genos")
		path := filepath.Join(dir, "credentials.json")
		source, err := Save(origin, "saved-token-abcdefghij", &fakeKeyring{setErr: errors.New("no keyring")}, path)
		if err != nil {
			t.Fatal(err)
		}
		if source != SourceFile {
			t.Fatalf("source = %s", source)
		}
		dirInfo, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode %o", dirInfo.Mode().Perm())
		}
		assertFileToken(t, path, origin, "saved-token-abcdefghij")
	})

	t.Run("nil keyring writes file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "credentials.json")
		source, err := Save(origin, "saved-token-abcdefghij", nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if source != SourceFile {
			t.Fatalf("source = %s", source)
		}
		assertFileToken(t, path, origin, "saved-token-abcdefghij")
	})

	t.Run("existing directory mode is unchanged", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "genos")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "credentials.json")
		if _, err := Save(origin, "saved-token-abcdefghij", nil, path); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("dir mode changed to %o", info.Mode().Perm())
		}
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("file mode %o", fileInfo.Mode().Perm())
		}
	})

	t.Run("merges hosts and refuses loose file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "credentials.json")
		other := "http://genos.localhost:8000"
		writeCredentials(t, path, other, "other-token-abcdefghij", 0o600)
		if _, err := Save(origin, "saved-token-abcdefghij", nil, path); err != nil {
			t.Fatal(err)
		}
		assertFileToken(t, path, origin, "saved-token-abcdefghij")
		assertFileToken(t, path, other, "other-token-abcdefghij")

		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Save(origin, "new-token-abcdefghij", nil, path)
		if err == nil || !strings.Contains(err.Error(), "chmod 0600") {
			t.Fatalf("error = %v", err)
		}
	})
}

func writeCredentials(t *testing.T, path, host, token string, mode os.FileMode) {
	t.Helper()
	doc := fileDocument{Hosts: map[string]hostEntry{host: {Token: token}}}
	payload, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, path, string(payload), mode)
}

func writeRaw(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertFileToken(t *testing.T, path, host, token string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc fileDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Hosts[host].Token != token {
		t.Fatalf("host %s token = %q", host, doc.Hosts[host].Token)
	}
}
