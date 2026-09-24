//go:build linux

package creds

import (
	"os/exec"
	"strings"
	"testing"

	ss "github.com/zalando/go-keyring/secret_service"
)

// secretServiceAvailable reports whether org.freedesktop.secrets is reachable
// on the session bus. CI sandboxes and headless boxes often lack it.
func secretServiceAvailable() bool {
	svc, err := ss.NewSecretService()
	if err != nil {
		return false
	}
	_, err = svc.OpenSession()
	return err == nil
}

func TestSystemKeyringHostAttributeRoundTrip(t *testing.T) {
	if !secretServiceAvailable() {
		t.Skip("Secret Service / D-Bus unavailable (org.freedesktop.secrets)")
	}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("secret-tool not on PATH")
	}

	origin := "http://127.0.0.1:9"
	secret := "genos_pat_roundtripcheck"
	ring := SystemKeyring{}
	if err := ring.Set(origin, secret); err != nil {
		t.Fatalf("set: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("secret-tool", "clear", "service", "genos", "host", origin).Run()
	})

	got, err := ring.Get(origin)
	if err != nil {
		t.Fatal(err)
	}
	if got != secret {
		t.Fatalf("Get = %q", got)
	}

	out, err := exec.Command("secret-tool", "lookup", "service", "genos", "host", origin).Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(string(out), "\n") != secret {
		t.Fatalf("secret-tool lookup = %q", out)
	}
	username, err := exec.Command("secret-tool", "lookup", "service", "genos", "username", origin).Output()
	if err == nil && len(username) > 0 {
		t.Fatalf("username attribute also stored the token: %q", username)
	}
}
