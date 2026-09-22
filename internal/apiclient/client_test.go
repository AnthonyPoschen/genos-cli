package apiclient

import (
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
