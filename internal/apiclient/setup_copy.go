package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SetupCopyDestination is one eligible destination from setup-copy-destinations.
type SetupCopyDestination struct {
	ServerID string             `json:"serverID"`
	Name     string             `json:"name"`
	Status   string             `json:"status"`
	Capacity SavedSetupCapacity `json:"capacity"`
	Eligible bool               `json:"eligible"`
	Reason   string             `json:"reason,omitempty"`
}

// SetupCopy is one setup copy/transfer job.
type SetupCopy struct {
	ID                  string     `json:"id"`
	SourceServerID      string     `json:"sourceServerID"`
	SourceSetupID       string     `json:"sourceSetupID"`
	DestinationServerID string     `json:"destinationServerID"`
	DestinationSetupID  string     `json:"destinationSetupID,omitempty"`
	GameID              string     `json:"gameID"`
	Mode                string     `json:"mode"`
	Status              string     `json:"status"`
	ProgressPercent     int32      `json:"progressPercent"`
	Stage               string     `json:"stage,omitempty"`
	Message             string     `json:"message,omitempty"`
	ArchiveName         string     `json:"archiveName,omitempty"`
	ArchiveBytes        int64      `json:"archiveBytes,omitempty"`
	SourceDeleted       bool       `json:"sourceDeleted"`
	LeavesSourceIntact  bool       `json:"leavesSourceIntact"`
	DestinationSelected bool       `json:"destinationSelected"`
	RequestedAt         time.Time  `json:"requestedAt"`
	FinishedAt          *time.Time `json:"finishedAt,omitempty"`
}

// SetupCopyTerminal reports whether status is a terminal copy status.
func SetupCopyTerminal(status string) bool {
	switch status {
	case "succeeded", "failed":
		return true
	default:
		return false
	}
}

// SetupCopyFailed reports whether status is a non-success terminal copy status.
func SetupCopyFailed(status string) bool {
	return status == "failed"
}

func decodeSetupCopy(data []byte) (SetupCopy, error) {
	var payload struct {
		Copy SetupCopy `json:"copy"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return SetupCopy{}, fmt.Errorf("setup copy response: %w", err)
	}
	if payload.Copy.ID == "" {
		return SetupCopy{}, errors.New("setup copy response missing copy")
	}
	return payload.Copy, nil
}

// ListSetupCopyDestinations calls GET /api/v1/servers/{id}/setup-copy-destinations.
func (c *Client) ListSetupCopyDestinations(ctx context.Context, serverID string) ([]SetupCopyDestination, error) {
	path := "/api/v1/servers/" + url.PathEscape(serverID) + "/setup-copy-destinations"
	data, status, err := c.do(ctx, http.MethodGet, path, nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Destinations []SetupCopyDestination `json:"destinations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("setup copy destinations response: %w", err)
	}
	if payload.Destinations == nil {
		payload.Destinations = []SetupCopyDestination{}
	}
	return payload.Destinations, nil
}

// StartSetupCopy calls POST /api/v1/servers/{id}/setups/{setupID}/copies.
// Expects 202 {"copy":...}. mode must be copy or transfer.
func (c *Client) StartSetupCopy(ctx context.Context, serverID, setupID, destinationServerID, mode string) (SetupCopy, error) {
	body := struct {
		DestinationServerID string `json:"destinationServerID"`
		Mode                string `json:"mode"`
	}{
		DestinationServerID: destinationServerID,
		Mode:                mode,
	}
	path := "/api/v1/servers/" + url.PathEscape(serverID) + "/setups/" + url.PathEscape(setupID) + "/copies"
	data, status, err := c.do(ctx, http.MethodPost, path, body, false)
	if err != nil {
		return SetupCopy{}, err
	}
	if status < 200 || status >= 300 {
		return SetupCopy{}, apiError(status, data)
	}
	return decodeSetupCopy(data)
}

// GetSetupCopy calls GET /api/v1/servers/{id}/setup-copies/{copyID}.
func (c *Client) GetSetupCopy(ctx context.Context, serverID, copyID string) (SetupCopy, error) {
	path := "/api/v1/servers/" + url.PathEscape(serverID) + "/setup-copies/" + url.PathEscape(copyID)
	data, status, err := c.do(ctx, http.MethodGet, path, nil, false)
	if err != nil {
		return SetupCopy{}, err
	}
	if status < 200 || status >= 300 {
		return SetupCopy{}, apiError(status, data)
	}
	return decodeSetupCopy(data)
}

// WaitSetupCopy polls GetSetupCopy until a terminal status, deadline, or ctx cancel.
func (c *Client) WaitSetupCopy(ctx context.Context, serverID, copyID string, interval time.Duration, deadline time.Time, sleep func(time.Duration), report func(SetupCopy)) (SetupCopy, error) {
	if sleep == nil {
		sleep = time.Sleep
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	var last SetupCopy
	for {
		copyJob, err := c.GetSetupCopy(ctx, serverID, copyID)
		if err != nil {
			return SetupCopy{}, err
		}
		last = copyJob
		if report != nil {
			report(copyJob)
		}
		if SetupCopyTerminal(copyJob.Status) {
			return copyJob, nil
		}
		if !time.Now().Before(deadline) {
			return last, fmt.Errorf("setup copy timed out with status %s", last.Status)
		}
		wait := interval
		if remain := time.Until(deadline); wait > remain {
			wait = remain
		}
		if wait > 0 {
			sleep(wait)
		}
		if err := ctx.Err(); err != nil {
			return last, err
		}
	}
}
