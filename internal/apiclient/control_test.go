package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenameCreateRenameDeleteSetupAndBroadcastHTTP(t *testing.T) {
	var gotMethod, gotAuth, gotKey, gotBody, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		gotMethod, gotQuery = r.Method, r.URL.RawQuery
		gotAuth, gotKey, gotBody = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), string(payload)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/servers/srv-1":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Renamed","game":{"name":"Factorio"},"status":"Stopped"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/setups":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"setup":{"id":"setup-new","name":"Factorio","game":{"name":"Factorio"}},"capacity":{"used":2,"limit":5},"server":{"id":"srv-1","name":"Alpha","status":"Stopped"}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/servers/srv-1/setups/setup-a":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Alpha","status":"Stopped"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/servers/srv-1/setups/setup-a":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","name":"Alpha","status":"Stopped"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/broadcast":
			_, _ = io.WriteString(w, `{"supported":true,"availability":"available","message":"hello","preview":"hello"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/broadcast":
			_, _ = io.WriteString(w, `{"supported":true,"availability":"available","delivered":true,"message":"hello"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	renamed, err := client.RenameServer(ctx, "srv-1", "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Renamed" || gotMethod != http.MethodPatch || gotKey != "" || gotAuth != "Bearer tok" {
		t.Fatalf("rename meta method=%s key=%q auth=%q server=%+v", gotMethod, gotKey, gotAuth, renamed)
	}
	if !strings.Contains(gotBody, `"name":"Renamed"`) {
		t.Fatalf("rename body %s", gotBody)
	}

	created, err := client.CreateSetup(ctx, "srv-1", "factorio")
	if err != nil {
		t.Fatal(err)
	}
	if created.Setup.ID != "setup-new" || created.Capacity.Used != 2 || created.Server.ID != "srv-1" {
		t.Fatalf("created %+v", created)
	}
	if gotMethod != http.MethodPost || gotKey != "" || !strings.Contains(gotBody, `"gameID":"factorio"`) {
		t.Fatalf("create method=%s key=%q body=%s", gotMethod, gotKey, gotBody)
	}

	mutated, err := client.RenameSetup(ctx, "srv-1", "setup-a", "Main factory", "Factorio", "setup-a")
	if err != nil {
		t.Fatal(err)
	}
	if mutated.Server.ID != "srv-1" || gotMethod != http.MethodPatch || gotKey != "" {
		t.Fatalf("rename-setup method=%s key=%q result=%+v", gotMethod, gotKey, mutated)
	}
	if !strings.Contains(gotBody, `"name":"Main factory"`) || !strings.Contains(gotBody, `"expectedName":"Factorio"`) || !strings.Contains(gotBody, `"expectedSelectedSetupID":"setup-a"`) {
		t.Fatalf("rename-setup body %s", gotBody)
	}

	mutated, err = client.DeleteSetup(ctx, "srv-1", "setup-a", "Main factory", "setup-a")
	if err != nil {
		t.Fatal(err)
	}
	if mutated.Server.ID != "srv-1" || gotMethod != http.MethodDelete || gotKey != "" {
		t.Fatalf("delete-setup method=%s key=%q result=%+v", gotMethod, gotKey, mutated)
	}
	if !strings.Contains(gotBody, `"expectedName":"Main factory"`) || !strings.Contains(gotBody, `"expectedSelectedSetupID":"setup-a"`) {
		t.Fatalf("delete-setup body %s", gotBody)
	}

	preview, err := client.GetBroadcast(ctx, "srv-1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(preview), `"availability":"available"`) || gotMethod != http.MethodGet || gotKey != "" || gotQuery != "message=hello" {
		t.Fatalf("get broadcast method=%s key=%q query=%q body=%s", gotMethod, gotKey, gotQuery, preview)
	}

	sent, err := client.SendBroadcast(ctx, "srv-1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sent), `"delivered":true`) || gotMethod != http.MethodPost || gotKey != "" || !strings.Contains(gotBody, `"message":"hello"`) {
		t.Fatalf("send broadcast method=%s key=%q req=%s resp=%s", gotMethod, gotKey, gotBody, sent)
	}
}

func TestRenameServerSurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"the Server must be confirmed stopped before changing its name"}}`)
	}))
	defer server.Close()

	_, err := New(server.URL, "tok").RenameServer(context.Background(), "srv-1", "Nope")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "server_not_confirmed_stopped" {
		t.Fatalf("err %#v", err)
	}
}

func TestBroadcastSurfacesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"broadcast_rate_limited","message":"Wait a moment before sending another notice."}}`)
	}))
	defer server.Close()

	_, err := New(server.URL, "tok").SendBroadcast(context.Background(), "srv-1", "hi")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "broadcast_rate_limited" {
		t.Fatalf("err %#v", err)
	}
}
