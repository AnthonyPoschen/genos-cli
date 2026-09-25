package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApprovalURL(t *testing.T) {
	const origin = "https://genosservers.com"
	const secret = "super-secret-access-token-value"
	tests := []struct {
		path    string
		want    string
		wantErr string
	}{
		{path: "/device?user_code=ABCD-EFGH", want: origin + "/device?user_code=ABCD-EFGH"},
		{path: "device?user_code=ABCD-EFGH", want: origin + "/device?user_code=ABCD-EFGH"},
		{path: origin + "/approve?code=ABCD", want: origin + "/approve?code=ABCD"},
		{path: "https://evil.example/steal?token=" + secret, wantErr: "does not match"},
		{path: "", wantErr: "empty"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got, err := ApprovalURL(origin, test.path)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ApprovalURL error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
			if strings.Contains(got, secret) {
				t.Fatalf("approval URL contains access token: %s", got)
			}
		})
	}
}

func TestDecodeServerShapes(t *testing.T) {
	bare := []byte(`{"id":"srv-1","name":"Alpha","game":{"name":"Factorio"},"status":"Running","metrics":{"playerCount":2}}`)
	wrapped := []byte(`{"server":{"id":"srv-1","name":"Alpha","game":{"name":"Factorio"},"status":"Stopped"}}`)
	for _, data := range [][]byte{bare, wrapped} {
		server, err := decodeServer(data)
		if err != nil {
			t.Fatal(err)
		}
		if server.ID != "srv-1" || server.Name != "Alpha" || server.Game.Name != "Factorio" {
			t.Fatalf("server = %+v", server)
		}
	}
	server, err := decodeServer(bare)
	if err != nil {
		t.Fatal(err)
	}
	if server.Metrics == nil || server.Metrics.PlayerCount == nil || *server.Metrics.PlayerCount != 2 {
		t.Fatalf("metrics = %+v", server.Metrics)
	}
}

func TestAPIErrorSurfacesCode(t *testing.T) {
	err := &APIError{Status: 409, Code: "server_not_confirmed_stopped", Message: "the Server must be confirmed stopped before changing Profiles"}
	got := err.Error()
	if !strings.Contains(got, "server_not_confirmed_stopped") || !strings.Contains(got, "confirmed stopped") {
		t.Fatalf("Error() = %q", got)
	}
}

func TestListSelectUnloadSetupsHTTP(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotKey, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth, gotKey, gotBody = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), string(payload)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/setups":
			_, _ = io.WriteString(w, `{"setups":[{"id":"setup-a","name":"Main","game":{"name":"Factorio"}}],"selectedSetupID":"setup-a","capacity":{"used":1,"limit":5},"creatableGames":[{"id":"factorio","name":"Factorio"}]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/selected-setup":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1","selectedSetupID":"setup-b"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/servers/srv-1/selected-setup":
			_, _ = io.WriteString(w, `{"server":{"id":"srv-1"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	chooser, err := client.ListSetups(ctx, "srv-1")
	if err != nil {
		t.Fatal(err)
	}
	if chooser.SelectedSetupID != "setup-a" || len(chooser.Setups) != 1 || chooser.Setups[0].Name != "Main" {
		t.Fatalf("chooser %+v", chooser)
	}
	if gotMethod != http.MethodGet || gotAuth != "Bearer tok" || gotKey != "" {
		t.Fatalf("list meta %s %s key=%q", gotMethod, gotAuth, gotKey)
	}

	if err := client.SelectSetup(ctx, "srv-1", "setup-b", "setup-a"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotKey != "" || !strings.Contains(gotBody, `"setupID":"setup-b"`) || !strings.Contains(gotBody, `"expectedSelectedSetupID":"setup-a"`) {
		t.Fatalf("select %s key=%q body=%s", gotMethod, gotKey, gotBody)
	}

	if err := client.UnloadSetup(ctx, "srv-1", "setup-b"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/servers/srv-1/selected-setup" || gotKey != "" || !strings.Contains(gotBody, `"expectedSelectedSetupID":"setup-b"`) {
		t.Fatalf("unload %s %s key=%q body=%s", gotMethod, gotPath, gotKey, gotBody)
	}
}

func TestConfigurationAndSchemaHTTP(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotKey, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth, gotKey, gotBody = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), string(payload)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/configuration":
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","name":"Main","gameID":"factorio","gameName":"Factorio","serverID":"srv-1","serverName":"Alpha","selected":true,"version":"v1","values":{"name":"Alpha"},"editable":true,"updatedAt":"2026-01-02T03:04:05Z","createdAt":"2026-01-01T00:00:00Z","secrets":{"version":"s1","configured":true,"pending":false}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/configuration":
			_, _ = io.WriteString(w, `{"configuration":{"setupID":"setup-a","version":"v1","values":{"name":"Beta"},"editable":true,"updatedAt":"2026-01-02T04:00:00Z","secrets":{"version":"s1","configured":true,"pending":false}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/games/factorio/management-schema":
			_, _ = io.WriteString(w, `{"gameID":"factorio","configuration":{"sections":[{"id":"broadcast"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	configuration, err := client.GetConfiguration(ctx, "srv-1")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SetupID != "setup-a" || configuration.Version != "v1" || configuration.GameID != "factorio" {
		t.Fatalf("configuration %+v", configuration)
	}
	if gotMethod != http.MethodGet || gotAuth != "Bearer tok" || gotKey != "" {
		t.Fatalf("get meta %s %s key=%q", gotMethod, gotAuth, gotKey)
	}

	body := map[string]any{
		"expectedSetupID":   "setup-a",
		"expectedUpdatedAt": "2026-01-02T03:04:05Z",
		"version":           "v1",
		"values":            map[string]any{"name": "Beta"},
	}
	updated, err := client.PutConfiguration(ctx, "srv-1", body)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.Values) != `{"name":"Beta"}` {
		t.Fatalf("updated values %s", updated.Values)
	}
	if gotMethod != http.MethodPut || gotKey != "" || !strings.Contains(gotBody, `"expectedSetupID":"setup-a"`) || !strings.Contains(gotBody, `"values"`) {
		t.Fatalf("put %s key=%q body=%s", gotMethod, gotKey, gotBody)
	}

	schema, err := client.GetManagementSchema(ctx, "factorio")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), `"gameID":"factorio"`) {
		t.Fatalf("schema %s", schema)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/games/factorio/management-schema" || gotKey != "" {
		t.Fatalf("schema meta %s %s key=%q", gotMethod, gotPath, gotKey)
	}
}

func TestPutConfigurationSurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"the Server must be confirmed stopped before changing configuration"}}`)
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	_, err := client.PutConfiguration(context.Background(), "srv-1", map[string]any{
		"expectedSetupID":   "setup-a",
		"expectedUpdatedAt": "2026-01-02T03:04:05Z",
		"version":           "v1",
		"values":            map[string]any{"name": "Beta"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "server_not_confirmed_stopped" {
		t.Fatalf("err %#v", err)
	}
}
