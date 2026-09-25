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

func TestModsHTTP(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth, gotKey, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		gotAuth, gotKey, gotBody = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key"), string(payload)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":true,"staged":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod","name":"tiny-mod","stageID":"stage-sha"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods/catalog":
			_, _ = io.WriteString(w, `{"catalog":{"query":"tiny","page":1,"pageSize":10,"results":[{"providerID":"factorio-mod-portal","providerModID":"tiny-mod","name":"tiny-mod","title":"Tiny Mod"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/mods/catalog/tiny-mod":
			_, _ = io.WriteString(w, `{"mod":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod","name":"tiny-mod","title":"Tiny Mod","latestVersion":"1.2.3"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/mods/credentials":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":true}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/servers/srv-1/mods/credentials":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":false}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/mods/staged-selection":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"credentialsConfigured":true,"staged":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod","stageID":"stage-sha"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/staged-removal":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"staged":{"operation":"remove","stageID":"stage-remove"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/discard":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/apply":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","gameID":"factorio","editable":true,"enabled":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	state, err := client.GetMods(ctx, "srv-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.SetupID != "setup-a" || state.Staged == nil || state.Staged.StageID != "stage-sha" {
		t.Fatalf("state %+v", state)
	}
	if gotMethod != http.MethodGet || gotAuth != "Bearer tok" || gotKey != "" {
		t.Fatalf("get meta %s %s key=%q", gotMethod, gotAuth, gotKey)
	}

	catalog, err := client.SearchModCatalog(ctx, "srv-1", ModCatalogQuery{Query: "tiny", Category: "logistics", Sort: "updated", Page: 2, PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(catalog), `"providerModID":"tiny-mod"`) {
		t.Fatalf("catalog %s", catalog)
	}
	if gotPath != "/api/v1/servers/srv-1/mods/catalog" || !strings.Contains(gotQuery, "q=tiny") || !strings.Contains(gotQuery, "page=2") || !strings.Contains(gotQuery, "pageSize=5") {
		t.Fatalf("search path=%s query=%s", gotPath, gotQuery)
	}

	detail, err := client.InspectModCatalog(ctx, "srv-1", "tiny-mod")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(detail), `"latestVersion":"1.2.3"`) {
		t.Fatalf("detail %s", detail)
	}

	state, err = client.SetModCredentials(ctx, "srv-1", "setup-a", "player", "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if !state.CredentialsConfigured || gotKey != "" || !strings.Contains(gotBody, `"username":"player"`) {
		t.Fatalf("credentials key=%q body=%s state=%+v", gotKey, gotBody, state)
	}

	state, err = client.ClearModCredentials(ctx, "srv-1", "setup-a")
	if err != nil {
		t.Fatal(err)
	}
	if state.CredentialsConfigured || gotMethod != http.MethodDelete || !strings.Contains(gotBody, `"expectedSetupID":"setup-a"`) {
		t.Fatalf("clear-credentials %s body=%s state=%+v", gotMethod, gotBody, state)
	}

	state, err = client.StageMod(ctx, "srv-1", "setup-a", "factorio-mod-portal", "tiny-mod")
	if err != nil {
		t.Fatal(err)
	}
	if state.Staged == nil || state.Staged.StageID != "stage-sha" || gotKey != "" {
		t.Fatalf("stage key=%q state=%+v", gotKey, state)
	}
	if !strings.Contains(gotBody, `"providerID":"factorio-mod-portal"`) || !strings.Contains(gotBody, `"providerModID":"tiny-mod"`) {
		t.Fatalf("stage body %s", gotBody)
	}

	state, err = client.UnstageMod(ctx, "srv-1", "setup-a")
	if err != nil {
		t.Fatal(err)
	}
	if state.Staged == nil || state.Staged.Operation != "remove" {
		t.Fatalf("unstage %+v", state)
	}

	state, err = client.DiscardMod(ctx, "srv-1", "setup-a")
	if err != nil {
		t.Fatal(err)
	}
	if state.Staged != nil {
		t.Fatalf("discard %+v", state)
	}

	state, err = client.ApplyMod(ctx, "srv-1", "setup-a", "stage-sha")
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled == nil || state.Enabled.ProviderModID != "tiny-mod" || gotKey != "" {
		t.Fatalf("apply key=%q state=%+v", gotKey, state)
	}
	if !strings.Contains(gotBody, `"stageID":"stage-sha"`) {
		t.Fatalf("apply body %s", gotBody)
	}
}

func TestModsApplySurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"mod changes can be applied only while the Server is confirmed stopped"}}`)
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	_, err := client.ApplyMod(context.Background(), "srv-1", "setup-a", "stage-sha")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "server_not_confirmed_stopped" {
		t.Fatalf("err %#v", err)
	}
}
