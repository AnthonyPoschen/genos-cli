package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CreateLibraryProfileResult is POST /api/v1/profiles (201).
type CreateLibraryProfileResult struct {
	Setup    SavedSetup         `json:"setup"`
	Capacity SavedSetupCapacity `json:"capacity"`
	Server   Server             `json:"server"`
}

// ProfileMutationResult is PATCH/DELETE /api/v1/profiles/{id}.
type ProfileMutationResult struct {
	Setup  *SavedSetup `json:"setup,omitempty"`
	Server *Server     `json:"server,omitempty"`
}

func profilesPath(profileID, suffix string) string {
	base := "/api/v1/profiles"
	if profileID == "" {
		return base + suffix
	}
	base += "/" + url.PathEscape(profileID)
	if suffix == "" {
		return base
	}
	return base + suffix
}

// LibraryProfileOptions calls GET /api/v1/profiles/games.
// Shape matches SetupChooser (creatableGames / capacity).
func (c *Client) LibraryProfileOptions(ctx context.Context) (SetupChooser, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/profiles/games", nil, false)
	if err != nil {
		return SetupChooser{}, err
	}
	if status < 200 || status >= 300 {
		return SetupChooser{}, apiError(status, data)
	}
	var chooser SetupChooser
	if err := json.Unmarshal(data, &chooser); err != nil {
		return SetupChooser{}, fmt.Errorf("library profile options response: %w", err)
	}
	if chooser.Setups == nil {
		chooser.Setups = []SavedSetup{}
	}
	if chooser.CreatableGames == nil {
		chooser.CreatableGames = []SetupGame{}
	}
	return chooser, nil
}

// CreateLibraryProfile calls POST /api/v1/profiles with {"gameID":...}.
func (c *Client) CreateLibraryProfile(ctx context.Context, gameID string) (CreateLibraryProfileResult, error) {
	body := struct {
		GameID string `json:"gameID"`
	}{GameID: gameID}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/profiles", body, false)
	if err != nil {
		return CreateLibraryProfileResult{}, err
	}
	if status < 200 || status >= 300 {
		return CreateLibraryProfileResult{}, apiError(status, data)
	}
	var result CreateLibraryProfileResult
	if err := json.Unmarshal(data, &result); err != nil {
		return CreateLibraryProfileResult{}, fmt.Errorf("create library profile response: %w", err)
	}
	return result, nil
}

// RenameLibraryProfile calls PATCH /api/v1/profiles/{id} with name + expectedName.
func (c *Client) RenameLibraryProfile(ctx context.Context, profileID, name, expectedName string) (ProfileMutationResult, error) {
	body := struct {
		Name         string `json:"name"`
		ExpectedName string `json:"expectedName"`
	}{Name: name, ExpectedName: expectedName}
	data, status, err := c.do(ctx, http.MethodPatch, profilesPath(profileID, ""), body, false)
	if err != nil {
		return ProfileMutationResult{}, err
	}
	if status < 200 || status >= 300 {
		return ProfileMutationResult{}, apiError(status, data)
	}
	var result ProfileMutationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return ProfileMutationResult{}, fmt.Errorf("rename library profile response: %w", err)
	}
	return result, nil
}

// DeleteLibraryProfile calls DELETE /api/v1/profiles/{id} with {"expectedName":...}.
func (c *Client) DeleteLibraryProfile(ctx context.Context, profileID, expectedName string) (ProfileMutationResult, error) {
	body := struct {
		ExpectedName string `json:"expectedName"`
	}{ExpectedName: expectedName}
	data, status, err := c.do(ctx, http.MethodDelete, profilesPath(profileID, ""), body, false)
	if err != nil {
		return ProfileMutationResult{}, err
	}
	if status < 200 || status >= 300 {
		return ProfileMutationResult{}, apiError(status, data)
	}
	var result ProfileMutationResult
	if len(data) > 0 {
		if err := json.Unmarshal(data, &result); err != nil {
			return ProfileMutationResult{}, fmt.Errorf("delete library profile response: %w", err)
		}
	}
	return result, nil
}

// GetProfileConfiguration calls GET /api/v1/profiles/{id}/configuration.
func (c *Client) GetProfileConfiguration(ctx context.Context, profileID string) (Configuration, error) {
	data, status, err := c.do(ctx, http.MethodGet, profilesPath(profileID, "/configuration"), nil, false)
	if err != nil {
		return Configuration{}, err
	}
	if status < 200 || status >= 300 {
		return Configuration{}, apiError(status, data)
	}
	var payload struct {
		Configuration Configuration `json:"configuration"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Configuration{}, fmt.Errorf("profile configuration response: %w", err)
	}
	return payload.Configuration, nil
}

// PutProfileConfiguration calls PUT /api/v1/profiles/{id}/configuration.
// body must already include expectedSetupID, expectedUpdatedAt, version, and values.
func (c *Client) PutProfileConfiguration(ctx context.Context, profileID string, body any) (Configuration, error) {
	data, status, err := c.do(ctx, http.MethodPut, profilesPath(profileID, "/configuration"), body, false)
	if err != nil {
		return Configuration{}, err
	}
	if status < 200 || status >= 300 {
		return Configuration{}, apiError(status, data)
	}
	var payload struct {
		Configuration Configuration `json:"configuration"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Configuration{}, fmt.Errorf("profile configuration response: %w", err)
	}
	return payload.Configuration, nil
}
