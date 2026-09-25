package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAwarenessAndProfilesClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/me":
			_, _ = io.WriteString(w, `{"id":"u1"}`)
		case r.URL.Path == "/api/v1/dashboard":
			_, _ = io.WriteString(w, `{"servers":[],"profiles":[{"id":"p1","name":"N"}],"profileCapacity":{"used":1,"limit":2}}`)
		case r.URL.Path == "/api/v1/catalog":
			_, _ = io.WriteString(w, `{"ok":true}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"setup":{"id":"p2","name":"Factorio"},"capacity":{"used":2,"limit":2}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, "tok")
	ctx := context.Background()
	if _, err := client.GetMeRaw(ctx); err != nil {
		t.Fatal(err)
	}
	dashboard, err := client.GetDashboard(ctx)
	if err != nil || dashboard.ProfileCapacity.Limit != 2 {
		t.Fatalf("%+v %v", dashboard, err)
	}
	if _, err := client.GetCatalogRaw(ctx); err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateLibraryProfile(ctx, "factorio")
	if err != nil || created.Setup.ID != "p2" {
		t.Fatalf("%+v %v", created, err)
	}
}

func TestSetupCopyWaitAndTokens(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/copies") && r.Method == http.MethodPost:
			if !strings.Contains(readBody(r), `"mode":"copy"`) {
				t.Fatalf("body")
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"copy":{"id":"c1","status":"pending"}}`)
		case strings.Contains(r.URL.Path, "/setup-copies/"):
			polls++
			if polls == 1 {
				_, _ = io.WriteString(w, `{"copy":{"id":"c1","status":"running"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"copy":{"id":"c1","status":"succeeded"}}`)
		case r.URL.Path == "/api/v1/auth/tokens" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"tokens":[]}`)
		case r.URL.Path == "/api/v1/auth/tokens" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":{"id":"t1"},"secret":"SECRET"}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, "tok")
	ctx := context.Background()
	started, err := client.StartSetupCopy(ctx, "s1", "setup", "s2", "copy")
	if err != nil {
		t.Fatal(err)
	}
	final, err := client.WaitSetupCopy(ctx, "s1", started.ID, time.Millisecond, time.Now().Add(time.Second), func(time.Duration) {}, nil)
	if err != nil || final.Status != "succeeded" {
		t.Fatalf("%+v %v", final, err)
	}
	if _, err := client.ListAccessTokens(ctx); err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAccessToken(ctx, "n", "c", "m")
	if err != nil || created.Secret != "SECRET" {
		t.Fatalf("%+v %v", created, err)
	}
	if err := client.RevokeAccessToken(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
}

func readBody(r *http.Request) string {
	data, _ := io.ReadAll(r.Body)
	return string(data)
}
