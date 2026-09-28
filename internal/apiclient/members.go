package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type ServerMember struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	ManageProfiles bool   `json:"manageProfiles"`
	Permission     string `json:"permission"`
}

type ServerInvite struct {
	ID             string `json:"id"`
	ServerID       string `json:"serverID"`
	ServerName     string `json:"serverName"`
	OwnerEmail     string `json:"ownerEmail"`
	ManageProfiles bool   `json:"manageProfiles"`
	Permission     string `json:"permission"`
}

func (c *Client) ListServerMembers(ctx context.Context, serverID string) ([]ServerMember, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/servers/"+url.PathEscape(serverID)+"/members", nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Members []ServerMember `json:"members"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("members response: %w", err)
	}
	return payload.Members, nil
}

func (c *Client) AddServerMember(ctx context.Context, serverID string, email string, manageProfiles bool) (ServerMember, error) {
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/servers/"+url.PathEscape(serverID)+"/members", map[string]any{"email": email, "manageProfiles": manageProfiles}, false)
	if err != nil {
		return ServerMember{}, err
	}
	if status < 200 || status >= 300 {
		return ServerMember{}, apiError(status, data)
	}
	var payload struct {
		Member ServerMember `json:"member"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ServerMember{}, fmt.Errorf("add member response: %w", err)
	}
	return payload.Member, nil
}

func (c *Client) RemoveServerMember(ctx context.Context, serverID string, memberID string) error {
	data, status, err := c.do(ctx, http.MethodDelete, "/api/v1/servers/"+url.PathEscape(serverID)+"/members/"+url.PathEscape(memberID), nil, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return apiError(status, data)
	}
	return nil
}

func (c *Client) SetServerMemberProfiles(ctx context.Context, serverID string, memberID string, manageProfiles bool) error {
	data, status, err := c.do(ctx, http.MethodPatch, "/api/v1/servers/"+url.PathEscape(serverID)+"/members/"+url.PathEscape(memberID), map[string]any{"manageProfiles": manageProfiles}, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return apiError(status, data)
	}
	return nil
}

func (c *Client) ListServerInvites(ctx context.Context) ([]ServerInvite, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/dashboard", nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Invites []ServerInvite `json:"pendingInvites"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("invites response: %w", err)
	}
	return payload.Invites, nil
}

func (c *Client) AnswerServerInvite(ctx context.Context, inviteID string, accept bool) error {
	action := "decline"
	if accept {
		action = "accept"
	}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/server-invites/"+url.PathEscape(inviteID)+"/"+action, map[string]any{}, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return apiError(status, data)
	}
	return nil
}

func (c *Client) LeaveServer(ctx context.Context, serverID string) error {
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/servers/"+url.PathEscape(serverID)+"/membership/leave", map[string]any{}, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return apiError(status, data)
	}
	return nil
}
