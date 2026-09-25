package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveExportCreateGetAndWait(t *testing.T) {
	var gets int
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-exports":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"export":{"id":"exp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"pending","progressPercent":0,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/save-exports/exp-1":
			gets++
			if gets < 3 {
				_, _ = io.WriteString(w, `{"export":{"id":"exp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"running","progressPercent":40,"stage":"upload","requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"export":{"id":"exp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"succeeded","progressPercent":100,"downloadURL":"https://files.example/save.zip","archiveName":"save.zip","archiveBytes":12,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	created, err := client.CreateSaveExport(ctx, "srv-1")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "exp-1" || created.Status != "pending" || gotKey != "" {
		t.Fatalf("create %+v key=%q", created, gotKey)
	}

	var reports []string
	final, err := client.WaitSaveExport(ctx, "srv-1", "exp-1", time.Millisecond, time.Now().Add(time.Minute), func(time.Duration) {}, func(export SaveExport) {
		reports = append(reports, export.Status)
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != "succeeded" || final.DownloadURL == "" || gets < 3 {
		t.Fatalf("final %+v gets=%d", final, gets)
	}
	if len(reports) < 2 || reports[len(reports)-1] != "succeeded" {
		t.Fatalf("reports %v", reports)
	}
}

func TestSaveExportWaitFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"export":{"id":"exp-9","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"failed","progressPercent":10,"message":"boom","requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	final, err := client.WaitSaveExport(context.Background(), "srv-1", "exp-9", time.Millisecond, time.Now().Add(time.Minute), func(time.Duration) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !SaveExportFailed(final.Status) {
		t.Fatalf("status %q", final.Status)
	}
}

func TestSaveImportCreateUploadValidateReplacePoll(t *testing.T) {
	var uploadAuth, uploadType, createKey, replaceBody string
	var gets int
	var uploadServer *httptest.Server
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports":
			createKey = r.Header.Get("Idempotency-Key")
			if !strings.Contains(string(payload), `"fileName":"world.zip"`) || !strings.Contains(string(payload), `"archiveBytes":4`) {
				t.Fatalf("create body %s", payload)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"import":{"id":"imp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"awaiting_upload","progressPercent":0,"uploadURL":"`+uploadServer.URL+`/put","fileName":"world.zip","mediaType":"application/zip","archiveBytes":4,"addonSetDiffers":false,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports/imp-1/validate":
			_, _ = io.WriteString(w, `{"import":{"id":"imp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"ready","progressPercent":40,"addonSetDiffers":true,"review":{"serverID":"srv-1","setupID":"setup-a","fileName":"world.zip","archiveBytes":4,"irreversible":true,"automaticRecoverySnapshot":true,"leavesServerStopped":true,"addonSetDiffers":true},"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/save-imports/imp-1/replace":
			replaceBody = string(payload)
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"import":{"id":"imp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"pending","progressPercent":50,"addonSetDiffers":true,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/save-imports/imp-1":
			gets++
			if gets < 2 {
				_, _ = io.WriteString(w, `{"import":{"id":"imp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"running","progressPercent":80,"addonSetDiffers":true,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"import":{"id":"imp-1","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"succeeded","progressPercent":100,"recovery":"snapshot-created","addonSetDiffers":true,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	uploadServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadAuth = r.Header.Get("Authorization")
		uploadType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || string(body) != "zip!" {
			http.Error(w, "bad upload", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadServer.Close()

	client := New(api.URL, "tok")
	ctx := context.Background()

	created, err := client.CreateSaveImport(ctx, "srv-1", "world.zip", "application/zip", 4)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "awaiting_upload" || created.UploadURL == "" || createKey != "" {
		t.Fatalf("create %+v key=%q", created, createKey)
	}
	if err := client.UploadSaveArchive(ctx, created.UploadURL, "application/zip", strings.NewReader("zip!"), 4); err != nil {
		t.Fatal(err)
	}
	if uploadAuth != "" || uploadType != "application/zip" {
		t.Fatalf("upload auth=%q type=%q", uploadAuth, uploadType)
	}

	validated, err := client.ValidateSaveImport(ctx, "srv-1", "imp-1")
	if err != nil {
		t.Fatal(err)
	}
	if validated.Status != "ready" || validated.Review == nil || !validated.Review.AddonSetDiffers {
		t.Fatalf("validate %+v", validated)
	}

	apply := true
	replaced, err := client.ReplaceSaveImport(ctx, "srv-1", "imp-1", true, &apply)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Status != "pending" || !strings.Contains(replaceBody, `"acknowledged":true`) || !strings.Contains(replaceBody, `"applySaveMods":true`) {
		t.Fatalf("replace %+v body=%s", replaced, replaceBody)
	}

	final, err := client.WaitSaveImport(ctx, "srv-1", "imp-1", time.Millisecond, time.Now().Add(time.Minute), func(time.Duration) {}, SaveImportTerminal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != "succeeded" || final.Recovery != "snapshot-created" {
		t.Fatalf("final %+v", final)
	}
}

func TestSaveImportReplaceOmitsApplyWhenNil(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"import":{"id":"imp-2","serverID":"srv-1","setupID":"setup-a","gameID":"factorio","status":"pending","progressPercent":50,"addonSetDiffers":false,"requestedAt":"2026-01-02T03:04:05Z","expiresAt":"2026-01-02T03:19:05Z"}}`)
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	_, err := client.ReplaceSaveImport(context.Background(), "srv-1", "imp-2", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"acknowledged":true`) || strings.Contains(body, "applySaveMods") {
		t.Fatalf("body %s", body)
	}
}

func TestDownloadToFileFollowsRedirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "SAVEBYTES")
	}))
	defer final.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/save.zip", http.StatusFound)
	}))
	defer redir.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.zip")
	client := New("http://example.invalid", "")
	if err := client.DownloadToFile(context.Background(), redir.URL+"/start", dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "SAVEBYTES" {
		t.Fatalf("got %q", data)
	}
}

func TestCreateSaveExportSurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"server_not_confirmed_stopped","message":"the server must be freshly confirmed stopped before exporting its save"}}`)
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	_, err := client.CreateSaveExport(context.Background(), "srv-1")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "server_not_confirmed_stopped" {
		t.Fatalf("err %#v", err)
	}
}
