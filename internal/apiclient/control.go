package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CreateSetupResult is POST /api/v1/servers/{id}/setups (201).
type CreateSetupResult struct {
	Setup    SavedSetup         `json:"setup"`
	Capacity SavedSetupCapacity `json:"capacity"`
	Server   Server             `json:"server"`
}

// SetupMutationResult is PATCH/DELETE /api/v1/servers/{id}/setups/{setupID}.
type SetupMutationResult struct {
	Server Server `json:"server"`
}

// RenameServer calls PATCH /api/v1/servers/{id} with {"name":...}.
// Idempotency-Key is not sent. The API requires confirmed Stopped; errors pass through.
func (c *Client) RenameServer(ctx context.Context, serverID, name string) (Server, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	data, status, err := c.do(ctx, http.MethodPatch, "/api/v1/servers/"+url.PathEscape(serverID), body, false)
	if err != nil {
		return Server{}, err
	}
	if status < 200 || status >= 300 {
		return Server{}, apiError(status, data)
	}
	return decodeServer(data)
}

// CreateSetup calls POST /api/v1/servers/{id}/setups with {"gameID":...}.
// Idempotency-Key is not sent.
func (c *Client) CreateSetup(ctx context.Context, serverID, gameID string) (CreateSetupResult, error) {
	body := struct {
		GameID string `json:"gameID"`
	}{GameID: gameID}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/servers/"+url.PathEscape(serverID)+"/setups", body, false)
	if err != nil {
		return CreateSetupResult{}, err
	}
	if status < 200 || status >= 300 {
		return CreateSetupResult{}, apiError(status, data)
	}
	var result CreateSetupResult
	if err := json.Unmarshal(data, &result); err != nil {
		return CreateSetupResult{}, fmt.Errorf("create setup response: %w", err)
	}
	return result, nil
}

// RenameSetup calls PATCH /api/v1/servers/{id}/setups/{setupID}.
// Idempotency-Key is not sent.
func (c *Client) RenameSetup(ctx context.Context, serverID, setupID, name, expectedName, expectedSelectedSetupID string) (SetupMutationResult, error) {
	body := struct {
		Name                    string `json:"name"`
		ExpectedName            string `json:"expectedName"`
		ExpectedSelectedSetupID string `json:"expectedSelectedSetupID"`
	}{
		Name:                    name,
		ExpectedName:            expectedName,
		ExpectedSelectedSetupID: expectedSelectedSetupID,
	}
	path := "/api/v1/servers/" + url.PathEscape(serverID) + "/setups/" + url.PathEscape(setupID)
	data, status, err := c.do(ctx, http.MethodPatch, path, body, false)
	if err != nil {
		return SetupMutationResult{}, err
	}
	if status < 200 || status >= 300 {
		return SetupMutationResult{}, apiError(status, data)
	}
	var result SetupMutationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return SetupMutationResult{}, fmt.Errorf("rename setup response: %w", err)
	}
	return result, nil
}

// DeleteSetup calls DELETE /api/v1/servers/{id}/setups/{setupID}.
// Idempotency-Key is not sent.
func (c *Client) DeleteSetup(ctx context.Context, serverID, setupID, expectedName, expectedSelectedSetupID string) (SetupMutationResult, error) {
	body := struct {
		ExpectedName            string `json:"expectedName"`
		ExpectedSelectedSetupID string `json:"expectedSelectedSetupID"`
	}{
		ExpectedName:            expectedName,
		ExpectedSelectedSetupID: expectedSelectedSetupID,
	}
	path := "/api/v1/servers/" + url.PathEscape(serverID) + "/setups/" + url.PathEscape(setupID)
	data, status, err := c.do(ctx, http.MethodDelete, path, body, false)
	if err != nil {
		return SetupMutationResult{}, err
	}
	if status < 200 || status >= 300 {
		return SetupMutationResult{}, apiError(status, data)
	}
	var result SetupMutationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return SetupMutationResult{}, fmt.Errorf("delete setup response: %w", err)
	}
	return result, nil
}

// GetBroadcast calls GET /api/v1/servers/{id}/broadcast.
// When message is non-empty it is sent as ?message=. Idempotency-Key is not sent.
func (c *Client) GetBroadcast(ctx context.Context, serverID, message string) (json.RawMessage, error) {
	path := "/api/v1/servers/" + url.PathEscape(serverID) + "/broadcast"
	if message != "" {
		path += "?message=" + url.QueryEscape(message)
	}
	data, status, err := c.do(ctx, http.MethodGet, path, nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("broadcast response is not valid JSON")
	}
	return json.RawMessage(data), nil
}

// SendBroadcast calls POST /api/v1/servers/{id}/broadcast with {"message":...}.
// message may be empty. Idempotency-Key is not sent.
func (c *Client) SendBroadcast(ctx context.Context, serverID, message string) (json.RawMessage, error) {
	body := struct {
		Message string `json:"message"`
	}{Message: message}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/servers/"+url.PathEscape(serverID)+"/broadcast", body, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("broadcast response is not valid JSON")
	}
	return json.RawMessage(data), nil
}
