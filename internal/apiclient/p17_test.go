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

func TestProfileModsHTTP(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		gotPath = r.URL.Path
		if strings.Contains(r.URL.Path, "/servers/") {
			t.Fatalf("unexpected servers path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profiles/prof-1/mods":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","gameID":"factorio","editable":true,"credentialsConfigured":true,"staged":{"providerID":"factorio-mod-portal","providerModID":"tiny-mod","stageID":"stage-sha"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profiles/prof-1/mods/catalog":
			if r.URL.Query().Get("q") != "tiny" {
				t.Fatalf("query %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"catalog":{"query":"tiny","results":[{"providerModID":"tiny-mod"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profiles/prof-1/mods/catalog/tiny-mod":
			_, _ = io.WriteString(w, `{"mod":{"providerModID":"tiny-mod","title":"Tiny"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/profiles/prof-1/mods/credentials":
			if !strings.Contains(string(payload), `"expectedSetupID":"prof-1"`) {
				t.Fatalf("body %s", payload)
			}
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","credentialsConfigured":true}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/profiles/prof-1/mods/credentials":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","credentialsConfigured":false}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/profiles/prof-1/mods/staged-selection":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","staged":{"stageID":"stage-sha","providerModID":"tiny-mod"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/mods/staged-removal":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","staged":{"operation":"remove","stageID":"stage-remove"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/mods/discard":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/mods/apply":
			if !strings.Contains(string(payload), `"stageID":"stage-sha"`) {
				t.Fatalf("apply body %s", payload)
			}
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","enabled":{"providerModID":"tiny-mod"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	state, err := client.GetProfileMods(ctx, "prof-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.SetupID != "prof-1" || gotPath != "/api/v1/profiles/prof-1/mods" {
		t.Fatalf("state %+v path %s", state, gotPath)
	}

	catalog, err := client.SearchProfileModCatalog(ctx, "prof-1", ModCatalogQuery{Query: "tiny"})
	if err != nil || !strings.Contains(string(catalog), "tiny-mod") {
		t.Fatalf("catalog %s err %v", catalog, err)
	}
	detail, err := client.InspectProfileModCatalog(ctx, "prof-1", "tiny-mod")
	if err != nil || !strings.Contains(string(detail), "Tiny") {
		t.Fatalf("detail %s err %v", detail, err)
	}
	if _, err := client.SetProfileModCredentials(ctx, "prof-1", "prof-1", "u", "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ClearProfileModCredentials(ctx, "prof-1", "prof-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StageProfileMod(ctx, "prof-1", "prof-1", "factorio-mod-portal", "tiny-mod"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UnstageProfileMod(ctx, "prof-1", "prof-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DiscardProfileMod(ctx, "prof-1", "prof-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApplyProfileMod(ctx, "prof-1", "prof-1", "stage-sha"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/profiles/prof-1/mods/apply" {
		t.Fatalf("last path %s", gotPath)
	}
}

func TestProfileSavesHTTP(t *testing.T) {
	var gotPath string
	var gets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if strings.Contains(r.URL.Path, "/servers/") {
			t.Fatalf("unexpected servers path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/save-exports":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"export":{"id":"exp-p","serverID":"","setupID":"prof-1","gameID":"factorio","status":"pending","progressPercent":0,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profiles/prof-1/save-exports/exp-p":
			gets++
			if gets < 2 {
				_, _ = io.WriteString(w, `{"export":{"id":"exp-p","setupID":"prof-1","gameID":"factorio","status":"running","progressPercent":50,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"export":{"id":"exp-p","setupID":"prof-1","gameID":"factorio","status":"succeeded","progressPercent":100,"downloadURL":"https://files.example/p.zip","requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/save-imports":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"import":{"id":"imp-p","setupID":"prof-1","gameID":"factorio","status":"awaiting_upload","progressPercent":0,"uploadURL":"https://upload.example/put","fileName":"world.zip","mediaType":"application/zip","archiveBytes":4,"addonSetDiffers":false,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/profiles/prof-1/save-imports/imp-p":
			_, _ = io.WriteString(w, `{"import":{"id":"imp-p","setupID":"prof-1","gameID":"factorio","status":"ready","progressPercent":40,"addonSetDiffers":false,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/save-imports/imp-p/validate":
			_, _ = io.WriteString(w, `{"import":{"id":"imp-p","setupID":"prof-1","gameID":"factorio","status":"ready","progressPercent":40,"addonSetDiffers":false,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/save-imports/imp-p/replace":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"import":{"id":"imp-p","setupID":"prof-1","gameID":"factorio","status":"running","progressPercent":70,"addonSetDiffers":false,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	created, err := client.CreateProfileSaveExport(ctx, "prof-1")
	if err != nil || created.ID != "exp-p" {
		t.Fatalf("create %+v err %v", created, err)
	}
	final, err := client.WaitProfileSaveExport(ctx, "prof-1", "exp-p", time.Millisecond, time.Now().Add(time.Minute), func(time.Duration) {}, nil)
	if err != nil || final.Status != "succeeded" || final.DownloadURL == "" {
		t.Fatalf("wait %+v err %v", final, err)
	}
	if gotPath != "/api/v1/profiles/prof-1/save-exports/exp-p" {
		t.Fatalf("path %s", gotPath)
	}

	imp, err := client.CreateProfileSaveImport(ctx, "prof-1", "world.zip", "application/zip", 4)
	if err != nil || imp.UploadURL == "" {
		t.Fatalf("import %+v err %v", imp, err)
	}
	if _, err := client.ValidateProfileSaveImport(ctx, "prof-1", "imp-p"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReplaceProfileSaveImport(ctx, "prof-1", "imp-p", true, nil); err != nil {
		t.Fatal(err)
	}
	got, err := client.GetProfileSaveImport(ctx, "prof-1", "imp-p")
	if err != nil || got.ID != "imp-p" {
		t.Fatalf("get %+v err %v", got, err)
	}
	if gotPath != "/api/v1/profiles/prof-1/save-imports/imp-p" {
		t.Fatalf("import path %s", gotPath)
	}
}
