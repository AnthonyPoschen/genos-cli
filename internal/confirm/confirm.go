// Package confirm decides whether a lifecycle action may be sent.
package confirm

import "fmt"

// Server is the customer-visible state the prompt rules look at.
type Server struct {
	Name           string
	PlayerCount    int64
	NotableUpdates []string
}

// Decision is the result of Confirm. Send is false when the caller must
// prompt and receive a yes before it performs the HTTP request.
type Decision struct {
	Send                       bool
	Prompt                     string
	ConfirmUnsavedProgressLoss bool
}

// Confirm applies the customer confirmation rules.
// yes is the --yes flag. An interactive yes answer is handled by the caller
// after a Decision with Send false and a non-empty Prompt.
func Confirm(action string, server Server, yes bool) (Decision, error) {
	switch action {
	case "start":
		return Decision{Send: true}, nil
	case "stop":
		if yes {
			return Decision{Send: true}, nil
		}
		return Decision{Prompt: fmt.Sprintf("Stop %s?", server.Name)}, nil
	case "force-stop":
		decision := Decision{ConfirmUnsavedProgressLoss: true}
		if yes {
			decision.Send = true
			return decision, nil
		}
		decision.Prompt = fmt.Sprintf("Force-stop %s? This loses unsaved progress.", server.Name)
		return decision, nil
	case "restart":
		if yes || !restartNeedsPrompt(server) {
			return Decision{Send: true}, nil
		}
		return Decision{Prompt: fmt.Sprintf("Restart %s?", server.Name)}, nil
	default:
		return Decision{}, fmt.Errorf("unknown action %q", action)
	}
}

func restartNeedsPrompt(server Server) bool {
	return server.PlayerCount > 0 || len(server.NotableUpdates) > 0
}
