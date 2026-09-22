package confirm

import (
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	idle := Server{Name: "Alpha"}
	players := Server{Name: "Alpha", PlayerCount: 2}
	updates := Server{Name: "Alpha", NotableUpdates: []string{"game build"}}
	blankUpdate := Server{Name: "Alpha", NotableUpdates: []string{""}}
	negativePlayers := Server{Name: "Alpha", PlayerCount: -1}

	tests := []struct {
		name       string
		action     string
		server     Server
		yes        bool
		wantSend   bool
		wantPrompt string
		wantSub    string
		wantFlag   bool
		wantErr    bool
	}{
		{name: "start ignores players", action: "start", server: players, wantSend: true},
		{name: "start with yes", action: "start", server: updates, yes: true, wantSend: true},
		{name: "stop prompts", action: "stop", server: idle, wantPrompt: "Stop Alpha?"},
		{name: "stop yes sends", action: "stop", server: idle, yes: true, wantSend: true},
		{name: "force-stop prompts", action: "force-stop", server: idle, wantSub: "unsaved progress", wantFlag: true},
		{name: "force-stop yes sends flag", action: "force-stop", server: players, yes: true, wantSend: true, wantFlag: true},
		{name: "restart idle sends", action: "restart", server: idle, wantSend: true},
		{name: "restart negative players sends", action: "restart", server: negativePlayers, wantSend: true},
		{name: "restart players prompts", action: "restart", server: players, wantSub: "Restart Alpha?"},
		{name: "restart updates prompts", action: "restart", server: updates, wantPrompt: "Restart Alpha?"},
		{name: "restart blank update prompts", action: "restart", server: blankUpdate, wantPrompt: "Restart Alpha?"},
		{name: "restart players yes sends", action: "restart", server: players, yes: true, wantSend: true},
		{name: "restart updates yes sends", action: "restart", server: updates, yes: true, wantSend: true},
		{name: "unknown action", action: "delete", server: idle, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Confirm(test.action, test.server, test.yes)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			if got.Send != test.wantSend {
				t.Fatalf("Send = %v, want %v", got.Send, test.wantSend)
			}
			if got.ConfirmUnsavedProgressLoss != test.wantFlag {
				t.Fatalf("ConfirmUnsavedProgressLoss = %v, want %v", got.ConfirmUnsavedProgressLoss, test.wantFlag)
			}
			if test.wantPrompt != "" && got.Prompt != test.wantPrompt {
				t.Fatalf("Prompt = %q, want %q", got.Prompt, test.wantPrompt)
			}
			if test.wantSub != "" && !strings.Contains(got.Prompt, test.wantSub) {
				t.Fatalf("Prompt = %q, want substring %q", got.Prompt, test.wantSub)
			}
			if test.wantSend && got.Prompt != "" {
				t.Fatalf("prompt set when sending: %q", got.Prompt)
			}
			if !test.wantSend && got.Prompt == "" {
				t.Fatal("expected a prompt when not sending")
			}
		})
	}
}
