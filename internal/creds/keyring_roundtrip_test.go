//go:build linux

package creds

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSystemKeyringHostAttributeRoundTrip(t *testing.T) {
	origin := "http://127.0.0.1:9"
	secret := "genos_pat_roundtripcheck"
	ring := SystemKeyring{}
	if err := ring.Set(origin, secret); err != nil {
		t.Fatalf("set: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("/usr/bin/secret-tool", "clear", "service", "genos", "host", origin).Run()
	})

	got, err := ring.Get(origin)
	if err != nil {
		t.Fatal(err)
	}
	if got != secret {
		t.Fatalf("Get = %q", got)
	}

	out, err := exec.Command("/usr/bin/secret-tool", "lookup", "service", "genos", "host", origin).Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(string(out), "\n") != secret {
		t.Fatalf("secret-tool lookup = %q", out)
	}
	username, err := exec.Command("/usr/bin/secret-tool", "lookup", "service", "genos", "username", origin).Output()
	if err == nil && len(username) > 0 {
		t.Fatalf("username attribute also stored the token: %q", username)
	}
}
