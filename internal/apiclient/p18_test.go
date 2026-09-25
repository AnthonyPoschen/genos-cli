package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestP18LeftoversHTTP(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(payload)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/public-rcon/credential":
			if !strings.Contains(string(payload), `"rotate":true`) && !strings.Contains(string(payload), `"rotate":false`) {
				t.Fatalf("body %s", payload)
			}
			_, _ = io.WriteString(w, `{"credential":"secret-once","connection":{"endpoint":"play.example:27020","port":27020},"notice":"Copy this password now. Genos will not show it again."}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/server-order":
			if !strings.Contains(string(payload), `"serverIDs":["srv-2","srv-1"]`) {
				t.Fatalf("body %s", payload)
			}
			_, _ = io.WriteString(w, `{"serverIDs":["srv-2","srv-1"]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers/srv-1/plan":
			_, _ = io.WriteString(w, `{"plan":{"offeringID":"standard","status":"active"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/mods/draft":
			if !strings.Contains(string(payload), `"expectedSetupID":"setup-a"`) ||
				!strings.Contains(string(payload), `"providerID":"factorio-mod-portal"`) ||
				!strings.Contains(string(payload), `"directModIDs":["tiny-mod","other-mod"]`) {
				t.Fatalf("draft body %s", payload)
			}
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","collection":{"draft":{"revision":3,"items":[{"providerModID":"tiny-mod"}]}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/servers/srv-1/mods/draft" && false:
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/import":
			if !strings.Contains(string(payload), `"content":"{\"mods\":[]}"`) {
				t.Fatalf("import body %s", payload)
			}
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","collection":{"draft":{"revision":4,"items":[]}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/servers/srv-1/mods/draft/apply":
			if !strings.Contains(string(payload), `"expectedRevision":3`) {
				t.Fatalf("apply body %s", payload)
			}
			_, _ = io.WriteString(w, `{"mods":{"setupID":"setup-a","collection":{"enabled":[{"providerModID":"tiny-mod"}]}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/profiles/prof-1/mods/draft":
			if !strings.Contains(string(payload), `"directModIDs":[]`) {
				t.Fatalf("empty draft body %s", payload)
			}
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","collection":{"draft":{"revision":1,"items":[]}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/mods/import":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1","collection":{"draft":{"revision":2,"items":[]}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/profiles/prof-1/mods/draft/apply":
			_, _ = io.WriteString(w, `{"mods":{"setupID":"prof-1"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "tok")
	ctx := context.Background()

	cred, err := client.RevealPublicRCON(ctx, "srv-1", false)
	if err != nil || cred.Credential != "secret-once" || gotPath != "/api/v1/servers/srv-1/public-rcon/credential" || gotMethod != http.MethodPost {
		t.Fatalf("public-rcon %+v err %v path %s method %s", cred, err, gotPath, gotMethod)
	}
	if !strings.Contains(gotBody, `"rotate":false`) {
		t.Fatalf("reveal body %s", gotBody)
	}
	cred, err = client.RevealPublicRCON(ctx, "srv-1", true)
	if err != nil || !strings.Contains(gotBody, `"rotate":true`) {
		t.Fatalf("rotate err %v body %s", err, gotBody)
	}

	ordered, err := client.ReplaceServerOrder(ctx, []string{"srv-2", "srv-1"})
	if err != nil || len(ordered) != 2 || ordered[0] != "srv-2" || gotPath != "/api/v1/server-order" || gotMethod != http.MethodPut {
		t.Fatalf("order %v err %v path %s", ordered, err, gotPath)
	}

	plan, err := client.GetServerPlan(ctx, "srv-1")
	if err != nil || !strings.Contains(string(plan), `"offeringID":"standard"`) || gotPath != "/api/v1/servers/srv-1/plan" {
		t.Fatalf("plan %s err %v path %s", plan, err, gotPath)
	}

	state, err := client.StageModDraft(ctx, "srv-1", "setup-a", "factorio-mod-portal", []string{"tiny-mod", "other-mod"})
	if err != nil || gotPath != "/api/v1/servers/srv-1/mods/draft" || gotMethod != http.MethodPut {
		t.Fatalf("draft %+v err %v path %s", state, err, gotPath)
	}
	rev, ok := state.DraftRevision()
	if !ok || rev != 3 {
		t.Fatalf("draft revision %d ok %v state %+v", rev, ok, state)
	}

	state, err = client.ImportModList(ctx, "srv-1", "setup-a", `{"mods":[]}`)
	if err != nil || gotPath != "/api/v1/servers/srv-1/mods/import" {
		t.Fatalf("import %+v err %v path %s", state, err, gotPath)
	}

	state, err = client.ApplyModDraft(ctx, "srv-1", "setup-a", 3)
	if err != nil || gotPath != "/api/v1/servers/srv-1/mods/draft/apply" {
		t.Fatalf("apply %+v err %v path %s", state, err, gotPath)
	}

	state, err = client.StageProfileModDraft(ctx, "prof-1", "prof-1", "factorio-mod-portal", nil)
	if err != nil || gotPath != "/api/v1/profiles/prof-1/mods/draft" || !strings.Contains(gotBody, `"directModIDs":[]`) {
		t.Fatalf("profile draft %+v err %v body %s path %s", state, err, gotBody, gotPath)
	}
	if _, err := client.ImportProfileModList(ctx, "prof-1", "prof-1", `{"mods":[]}`); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/profiles/prof-1/mods/import" {
		t.Fatalf("profile import path %s", gotPath)
	}
	if _, err := client.ApplyProfileModDraft(ctx, "prof-1", "prof-1", 2); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/profiles/prof-1/mods/draft/apply" {
		t.Fatalf("profile apply path %s", gotPath)
	}
}

func TestDraftRevisionMissing(t *testing.T) {
	state := SetupModState{Collection: json.RawMessage(`{"enabled":[]}`)}
	if _, ok := state.DraftRevision(); ok {
		t.Fatal("expected missing draft revision")
	}
	state.Collection = json.RawMessage(`{"draft":{"revision":0,"items":[]}}`)
	if _, ok := state.DraftRevision(); ok {
		t.Fatal("expected revision < 1 to be missing")
	}
}
