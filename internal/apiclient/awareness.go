package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DashboardProfile is one library profile summary from GET /api/v1/dashboard.
type DashboardProfile struct {
	ID           string `json:"id"`
	ServerID     string `json:"serverID"`
	ServerName   string `json:"serverName"`
	ServerStatus string `json:"serverStatus"`
	Game         Game   `json:"game"`
	Name         string `json:"name"`
	Selected     bool   `json:"selected"`
}

// Dashboard is GET /api/v1/dashboard.
type Dashboard struct {
	Servers         []Server             `json:"servers"`
	Profiles        []DashboardProfile   `json:"profiles"`
	ProfileCapacity SavedSetupCapacity   `json:"profileCapacity"`
}

// GetMeRaw calls GET /api/v1/me and returns the JSON body as returned.
func (c *Client) GetMeRaw(ctx context.Context) (json.RawMessage, error) {
	return c.getRawJSON(ctx, "/api/v1/me", false)
}

// GetDashboard calls GET /api/v1/dashboard.
func (c *Client) GetDashboard(ctx context.Context) (Dashboard, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/dashboard", nil, false)
	if err != nil {
		return Dashboard{}, err
	}
	if status < 200 || status >= 300 {
		return Dashboard{}, apiError(status, data)
	}
	var dashboard Dashboard
	if err := json.Unmarshal(data, &dashboard); err != nil {
		return Dashboard{}, fmt.Errorf("dashboard response: %w", err)
	}
	if dashboard.Profiles == nil {
		dashboard.Profiles = []DashboardProfile{}
	}
	if dashboard.Servers == nil {
		dashboard.Servers = []Server{}
	}
	return dashboard, nil
}

// GetDashboardRaw calls GET /api/v1/dashboard and returns the JSON body as returned.
func (c *Client) GetDashboardRaw(ctx context.Context) (json.RawMessage, error) {
	return c.getRawJSON(ctx, "/api/v1/dashboard", false)
}

// GetCatalogRaw calls GET /api/v1/catalog (public) and returns the JSON body as returned.
func (c *Client) GetCatalogRaw(ctx context.Context) (json.RawMessage, error) {
	return c.getRawJSON(ctx, "/api/v1/catalog", false)
}

func (c *Client) getRawJSON(ctx context.Context, path string, idempotency bool) (json.RawMessage, error) {
	data, status, err := c.do(ctx, http.MethodGet, path, nil, idempotency)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("%s response is not valid JSON", path)
	}
	return json.RawMessage(data), nil
}
