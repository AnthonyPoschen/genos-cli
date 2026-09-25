package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// PublicRCONCredential is POST /api/v1/servers/{id}/public-rcon/credential.
type PublicRCONCredential struct {
	Credential string             `json:"credential"`
	Connection PublicRCONEndpoint `json:"connection"`
	Notice     string             `json:"notice"`
}

// PublicRCONEndpoint is the connection block returned with a credential.
type PublicRCONEndpoint struct {
	Endpoint string `json:"endpoint,omitempty"`
	Port     int32  `json:"port,omitempty"`
}

// RevealPublicRCON calls POST /api/v1/servers/{id}/public-rcon/credential.
// rotate=false is the first reveal; rotate=true issues a new credential.
func (c *Client) RevealPublicRCON(ctx context.Context, serverID string, rotate bool) (PublicRCONCredential, error) {
	body := struct {
		Rotate bool `json:"rotate"`
	}{Rotate: rotate}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/servers/"+url.PathEscape(serverID)+"/public-rcon/credential", body, false)
	if err != nil {
		return PublicRCONCredential{}, err
	}
	if status < 200 || status >= 300 {
		return PublicRCONCredential{}, apiError(status, data)
	}
	var payload PublicRCONCredential
	if err := json.Unmarshal(data, &payload); err != nil {
		return PublicRCONCredential{}, fmt.Errorf("public-rcon response: %w", err)
	}
	return payload, nil
}

// ReplaceServerOrder calls PUT /api/v1/server-order.
// serverIDs must list every owned server exactly once.
func (c *Client) ReplaceServerOrder(ctx context.Context, serverIDs []string) ([]string, error) {
	if serverIDs == nil {
		serverIDs = []string{}
	}
	body := struct {
		ServerIDs []string `json:"serverIDs"`
	}{ServerIDs: serverIDs}
	data, status, err := c.do(ctx, http.MethodPut, "/api/v1/server-order", body, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		ServerIDs []string `json:"serverIDs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("server-order response: %w", err)
	}
	return payload.ServerIDs, nil
}

// GetServerPlan calls GET /api/v1/servers/{id}/plan and returns the inner plan object.
func (c *Client) GetServerPlan(ctx context.Context, serverID string) (json.RawMessage, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/servers/"+url.PathEscape(serverID)+"/plan", nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Plan json.RawMessage `json:"plan"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("plan response: %w", err)
	}
	if len(payload.Plan) == 0 || string(payload.Plan) == "null" {
		return nil, fmt.Errorf("plan response missing plan")
	}
	return payload.Plan, nil
}
