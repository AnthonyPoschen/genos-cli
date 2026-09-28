package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerListShowsOwnerAndOmitsPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/servers" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"servers":[
			{"id":"srv-1","name":"Alpha","game":{"name":"Factorio"},"status":"Running","access":{"role":"owner","manageProfiles":true}},
			{"id":"srv-2","name":"Beta","game":{"name":"Factorio"},"status":"Stopped","access":{"role":"member","ownerEmail":"alex@example.com","manageProfiles":false}}
		]}`)
	}))
	defer server.Close()
	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	if code := Run([]string{"server", "list"}, opts); code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "OWNER") || !strings.Contains(out, "you") || !strings.Contains(out, "alex@example.com") {
		t.Fatalf("list = %q", out)
	}
	if strings.Contains(out, "pending") {
		t.Fatalf("pending row leaked: %q", out)
	}
}

func TestMemberAddLeaveAndPlanUseTheToken(t *testing.T) {
	var planPath, addPath, leavePath, configPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/members"):
			addPath = r.URL.Path
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"owner_only","message":"Only the owner can change who has access to this Server."}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/plan"):
			planPath = r.URL.Path
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"owner_only","message":"Only the owner can change billing."}}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/configuration"):
			configPath = r.URL.Path
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"profile_management_required","message":"You can start and stop this Server and change its mods. You cannot edit Profiles."}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/membership/leave"):
			leavePath = r.URL.Path
			_, _ = io.WriteString(w, `{"left":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	opts, stdout, stderr := testOptions(t.TempDir(), server.URL, testToken, nil)
	configBody := `{"expectedSetupID":"setup-1","expectedUpdatedAt":"2026-01-01T00:00:00Z","version":"factorio/v1","values":{"name":"World"}}`
	if code := Run([]string{"server", "member", "add", "srv-foreign", "owner@example.com"}, opts); code == 0 {
		t.Fatal("member add on someone else's server succeeded")
	}
	if addPath != "/api/v1/servers/srv-foreign/members" || !strings.Contains(stderr.String(), "Only the owner can change who has access") {
		t.Fatalf("add path %s stderr %s", addPath, stderr.String())
	}
	stderr.Reset()
	if code := Run([]string{"server", "plan", "srv-shared"}, opts); code == 0 {
		t.Fatal("plan command succeeded")
	}
	if planPath != "/api/v1/servers/srv-shared/plan" || !strings.Contains(stderr.String(), "Only the owner can change billing") {
		t.Fatalf("plan path %s stderr %s", planPath, stderr.String())
	}
	stderr.Reset()
	opts.Stdin = strings.NewReader(configBody)
	if code := Run([]string{"server", "config", "put", "srv-shared"}, opts); code == 0 {
		t.Fatal("configuration without profile management succeeded")
	}
	if !strings.Contains(stderr.String(), "cannot edit Profiles") {
		t.Fatalf("config stderr %s path %s", stderr.String(), configPath)
	}
	stdout.Reset()
	stderr.Reset()
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/configuration") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"configuration":{"version":"factorio/v1","values":{}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer allowed.Close()
	optsAllowed, stdoutAllowed, stderrAllowed := testOptions(t.TempDir(), allowed.URL, testToken, nil)
	optsAllowed.Stdin = strings.NewReader(configBody)
	if code := Run([]string{"server", "config", "put", "srv-shared"}, optsAllowed); code != 0 {
		t.Fatalf("configuration with profile management exit %d stderr %s", code, stderrAllowed.String())
	}
	if !strings.Contains(stdoutAllowed.String(), "factorio/v1") && stdoutAllowed.Len()+stderrAllowed.Len() == 0 {
		t.Fatalf("expected a configuration response, stdout %q stderr %q", stdoutAllowed.String(), stderrAllowed.String())
	}
	if code := Run([]string{"server", "leave", "srv-shared"}, opts); code != 0 {
		t.Fatalf("leave exit %d stderr %s", code, stderr.String())
	}
	if leavePath != "/api/v1/servers/srv-shared/membership/leave" || !strings.Contains(stdout.String(), "Left the server.") {
		t.Fatalf("leave path %s stdout %s", leavePath, stdout.String())
	}
}
