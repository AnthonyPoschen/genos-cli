package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// AccessToken is metadata for one customer PAT (never includes the secret).
type AccessToken struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userID"`
	TokenPrefix string     `json:"tokenPrefix"`
	Name        string     `json:"name"`
	ClientName  string     `json:"clientName"`
	MachineName string     `json:"machineName"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// CreateAccessTokenResult is POST /api/v1/auth/tokens (201).
// Secret is the plaintext token and is only available at creation.
type CreateAccessTokenResult struct {
	Token  AccessToken `json:"token"`
	Secret string      `json:"secret"`
}

// ListAccessTokens calls GET /api/v1/auth/tokens.
func (c *Client) ListAccessTokens(ctx context.Context) ([]AccessToken, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/auth/tokens", nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Tokens []AccessToken `json:"tokens"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("auth tokens response: %w", err)
	}
	if payload.Tokens == nil {
		payload.Tokens = []AccessToken{}
	}
	return payload.Tokens, nil
}

// CreateAccessToken calls POST /api/v1/auth/tokens.
// Returns metadata plus the plaintext secret (print once; do not log).
func (c *Client) CreateAccessToken(ctx context.Context, name, clientName, machineName string) (CreateAccessTokenResult, error) {
	body := struct {
		Name        string `json:"name"`
		ClientName  string `json:"clientName"`
		MachineName string `json:"machineName"`
	}{
		Name:        name,
		ClientName:  clientName,
		MachineName: machineName,
	}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/auth/tokens", body, false)
	if err != nil {
		return CreateAccessTokenResult{}, err
	}
	if status < 200 || status >= 300 {
		return CreateAccessTokenResult{}, apiError(status, data)
	}
	var result CreateAccessTokenResult
	if err := json.Unmarshal(data, &result); err != nil {
		return CreateAccessTokenResult{}, fmt.Errorf("create auth token response: %w", err)
	}
	if result.Secret == "" {
		return CreateAccessTokenResult{}, fmt.Errorf("create auth token response missing secret")
	}
	return result, nil
}

// RevokeAccessToken calls DELETE /api/v1/auth/tokens/{tokenID} (204).
func (c *Client) RevokeAccessToken(ctx context.Context, tokenID string) error {
	data, status, err := c.do(ctx, http.MethodDelete, "/api/v1/auth/tokens/"+url.PathEscape(tokenID), nil, false)
	if err != nil {
		return err
	}
	if status == http.StatusNoContent || (status >= 200 && status < 300) {
		return nil
	}
	return apiError(status, data)
}
