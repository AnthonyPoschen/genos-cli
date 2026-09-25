package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// SaveExport is one server save export job.
type SaveExport struct {
	ID              string     `json:"id"`
	ServerID        string     `json:"serverID"`
	SetupID         string     `json:"setupID"`
	GameID          string     `json:"gameID"`
	Status          string     `json:"status"`
	ProgressPercent int32      `json:"progressPercent"`
	Stage           string     `json:"stage,omitempty"`
	ArchiveName     string     `json:"archiveName,omitempty"`
	ArchiveBytes    int64      `json:"archiveBytes,omitempty"`
	Message         string     `json:"message,omitempty"`
	DownloadURL     string     `json:"downloadURL,omitempty"`
	RequestedAt     time.Time  `json:"requestedAt"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
}

// SaveImportAddon is one mod entry on a save import review.
type SaveImportAddon struct {
	ModID   string `json:"modID"`
	Version string `json:"version"`
}

// SaveImportReview is the validated import review payload.
type SaveImportReview struct {
	ServerID                  string            `json:"serverID"`
	ServerName                string            `json:"serverName"`
	SetupID                   string            `json:"setupID"`
	SetupName                 string            `json:"setupName"`
	FileName                  string            `json:"fileName"`
	ArchiveBytes              int64             `json:"archiveBytes"`
	Irreversible              bool              `json:"irreversible"`
	AutomaticRecoverySnapshot bool              `json:"automaticRecoverySnapshot"`
	LeavesServerStopped       bool              `json:"leavesServerStopped"`
	DiscoveredAddons          []SaveImportAddon `json:"discoveredAddons"`
	CurrentAddons             []SaveImportAddon `json:"currentAddons"`
	AddonSetDiffers           bool              `json:"addonSetDiffers"`
}

// SaveImport is one server save import job.
type SaveImport struct {
	ID               string            `json:"id"`
	ServerID         string            `json:"serverID"`
	SetupID          string            `json:"setupID"`
	GameID           string            `json:"gameID"`
	Status           string            `json:"status"`
	ProgressPercent  int32             `json:"progressPercent"`
	Stage            string            `json:"stage,omitempty"`
	FileName         string            `json:"fileName,omitempty"`
	MediaType        string            `json:"mediaType,omitempty"`
	ArchiveBytes     int64             `json:"archiveBytes,omitempty"`
	Message          string            `json:"message,omitempty"`
	Recovery         string            `json:"recovery,omitempty"`
	UploadURL        string            `json:"uploadURL,omitempty"`
	Review           *SaveImportReview `json:"review,omitempty"`
	DiscoveredAddons []SaveImportAddon `json:"discoveredAddons,omitempty"`
	CurrentAddons    []SaveImportAddon `json:"currentAddons,omitempty"`
	AddonSetDiffers  bool              `json:"addonSetDiffers"`
	ModSyncStatus    string            `json:"modSyncStatus,omitempty"`
	ModSyncMessage   string            `json:"modSyncMessage,omitempty"`
	RequestedAt      time.Time         `json:"requestedAt"`
	ExpiresAt        time.Time         `json:"expiresAt"`
	FinishedAt       *time.Time        `json:"finishedAt,omitempty"`
}

func savesPath(serverID, kind, id, action string) string {
	base := "/api/v1/servers/" + url.PathEscape(serverID) + "/" + kind
	if id == "" {
		return base
	}
	base += "/" + url.PathEscape(id)
	if action == "" {
		return base
	}
	return base + "/" + action
}

func decodeSaveExport(data []byte) (SaveExport, error) {
	var payload struct {
		Export SaveExport `json:"export"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return SaveExport{}, fmt.Errorf("save export response: %w", err)
	}
	if payload.Export.ID == "" {
		return SaveExport{}, errors.New("save export response missing export")
	}
	return payload.Export, nil
}

func decodeSaveImport(data []byte) (SaveImport, error) {
	var payload struct {
		Import SaveImport `json:"import"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return SaveImport{}, fmt.Errorf("save import response: %w", err)
	}
	if payload.Import.ID == "" {
		return SaveImport{}, errors.New("save import response missing import")
	}
	return payload.Import, nil
}

// CreateSaveExport calls POST /api/v1/servers/{id}/save-exports.
// Idempotency-Key is not sent (matches dashboard).
func (c *Client) CreateSaveExport(ctx context.Context, serverID string) (SaveExport, error) {
	data, status, err := c.do(ctx, http.MethodPost, savesPath(serverID, "save-exports", "", ""), nil, false)
	if err != nil {
		return SaveExport{}, err
	}
	if status < 200 || status >= 300 {
		return SaveExport{}, apiError(status, data)
	}
	return decodeSaveExport(data)
}

// GetSaveExport calls GET /api/v1/servers/{id}/save-exports/{exportID}.
func (c *Client) GetSaveExport(ctx context.Context, serverID, exportID string) (SaveExport, error) {
	data, status, err := c.do(ctx, http.MethodGet, savesPath(serverID, "save-exports", exportID, ""), nil, false)
	if err != nil {
		return SaveExport{}, err
	}
	if status < 200 || status >= 300 {
		return SaveExport{}, apiError(status, data)
	}
	return decodeSaveExport(data)
}

// SaveExportTerminal reports whether status is a terminal export status.
func SaveExportTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "expired":
		return true
	default:
		return false
	}
}

// SaveExportFailed reports whether status is a non-success terminal export status.
func SaveExportFailed(status string) bool {
	return status == "failed" || status == "expired"
}

// WaitSaveExport polls GetSaveExport until a terminal status, deadline, or ctx cancel.
// sleep may be nil. report may be nil; when set it is called after every successful GET.
func (c *Client) WaitSaveExport(ctx context.Context, serverID, exportID string, interval time.Duration, deadline time.Time, sleep func(time.Duration), report func(SaveExport)) (SaveExport, error) {
	if sleep == nil {
		sleep = time.Sleep
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		export, err := c.GetSaveExport(ctx, serverID, exportID)
		if err != nil {
			return SaveExport{}, err
		}
		if report != nil {
			report(export)
		}
		if SaveExportTerminal(export.Status) {
			return export, nil
		}
		if err := waitPoll(ctx, interval, deadline, sleep); err != nil {
			return export, err
		}
	}
}

// CreateSaveImport calls POST /api/v1/servers/{id}/save-imports.
// Idempotency-Key is not sent (matches dashboard).
func (c *Client) CreateSaveImport(ctx context.Context, serverID, fileName, mediaType string, archiveBytes int64) (SaveImport, error) {
	body := struct {
		FileName     string `json:"fileName"`
		MediaType    string `json:"mediaType"`
		ArchiveBytes int64  `json:"archiveBytes"`
	}{
		FileName:     fileName,
		MediaType:    mediaType,
		ArchiveBytes: archiveBytes,
	}
	data, status, err := c.do(ctx, http.MethodPost, savesPath(serverID, "save-imports", "", ""), body, false)
	if err != nil {
		return SaveImport{}, err
	}
	if status < 200 || status >= 300 {
		return SaveImport{}, apiError(status, data)
	}
	return decodeSaveImport(data)
}

// GetSaveImport calls GET /api/v1/servers/{id}/save-imports/{importID}.
func (c *Client) GetSaveImport(ctx context.Context, serverID, importID string) (SaveImport, error) {
	data, status, err := c.do(ctx, http.MethodGet, savesPath(serverID, "save-imports", importID, ""), nil, false)
	if err != nil {
		return SaveImport{}, err
	}
	if status < 200 || status >= 300 {
		return SaveImport{}, apiError(status, data)
	}
	return decodeSaveImport(data)
}

// ValidateSaveImport calls POST /api/v1/servers/{id}/save-imports/{importID}/validate.
func (c *Client) ValidateSaveImport(ctx context.Context, serverID, importID string) (SaveImport, error) {
	data, status, err := c.do(ctx, http.MethodPost, savesPath(serverID, "save-imports", importID, "validate"), nil, false)
	if err != nil {
		return SaveImport{}, err
	}
	if status < 200 || status >= 300 {
		return SaveImport{}, apiError(status, data)
	}
	return decodeSaveImport(data)
}

// ReplaceSaveImport calls POST /api/v1/servers/{id}/save-imports/{importID}/replace.
// applySaveMods is omitted from the JSON body when nil.
func (c *Client) ReplaceSaveImport(ctx context.Context, serverID, importID string, acknowledged bool, applySaveMods *bool) (SaveImport, error) {
	body := map[string]any{"acknowledged": acknowledged}
	if applySaveMods != nil {
		body["applySaveMods"] = *applySaveMods
	}
	data, status, err := c.do(ctx, http.MethodPost, savesPath(serverID, "save-imports", importID, "replace"), body, false)
	if err != nil {
		return SaveImport{}, err
	}
	if status < 200 || status >= 300 {
		return SaveImport{}, apiError(status, data)
	}
	return decodeSaveImport(data)
}

// UploadSaveArchive PUTs archive bytes to a pre-signed uploadURL.
// No Genos Authorization header is sent. contentType should be application/zip.
func (c *Client) UploadSaveArchive(ctx context.Context, uploadURL, contentType string, body io.Reader, contentLength int64) error {
	if strings.TrimSpace(uploadURL) == "" {
		return errors.New("save import upload URL is empty")
	}
	if contentType == "" {
		contentType = "application/zip"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", userAgent)
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	resp, err := c.transferHTTP(false).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("save import upload failed with status %d", resp.StatusCode)
	}
	return nil
}

// DownloadToFile GETs downloadURL (following redirects, no Genos auth) into destPath.
func (c *Client) DownloadToFile(ctx context.Context, downloadURL, destPath string) error {
	if strings.TrimSpace(downloadURL) == "" {
		return errors.New("save export download URL is empty")
	}
	if strings.TrimSpace(destPath) == "" {
		return errors.New("download destination path is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.transferHTTP(true).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("save export download failed with status %d", resp.StatusCode)
	}
	file, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}
	return file.Close()
}

// SaveImportReady reports whether the import is ready for replace.
func SaveImportReady(status string) bool {
	return status == "ready"
}

// SaveImportTerminal reports whether status is a terminal import replace status.
func SaveImportTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "expired":
		return true
	default:
		return false
	}
}

// SaveImportFailed reports whether status is a non-success terminal import status.
func SaveImportFailed(status string) bool {
	return status == "failed" || status == "expired"
}

// SaveImportReviewOrFailed is true when polling should stop before replace.
func SaveImportReviewOrFailed(status string) bool {
	return SaveImportReady(status) || SaveImportFailed(status)
}

// WaitSaveImport polls GetSaveImport until stop(status) is true, deadline, or ctx cancel.
func (c *Client) WaitSaveImport(ctx context.Context, serverID, importID string, interval time.Duration, deadline time.Time, sleep func(time.Duration), stop func(string) bool, report func(SaveImport)) (SaveImport, error) {
	if sleep == nil {
		sleep = time.Sleep
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if stop == nil {
		stop = SaveImportTerminal
	}
	for {
		replacement, err := c.GetSaveImport(ctx, serverID, importID)
		if err != nil {
			return SaveImport{}, err
		}
		if report != nil {
			report(replacement)
		}
		if stop(replacement.Status) {
			return replacement, nil
		}
		if err := waitPoll(ctx, interval, deadline, sleep); err != nil {
			return replacement, err
		}
	}
}

func waitPoll(ctx context.Context, interval time.Duration, deadline time.Time, sleep func(time.Duration)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return errors.New("timed out waiting for save operation")
	}
	wait := interval
	if !deadline.IsZero() {
		if remain := time.Until(deadline); wait > remain {
			wait = remain
		}
	}
	if wait > 0 {
		sleep(wait)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return errors.New("timed out waiting for save operation")
	}
	return nil
}

// transferHTTP returns an HTTP client for pre-signed upload/download URLs.
// followRedirects should be true for downloads and false for uploads.
func (c *Client) transferHTTP(followRedirects bool) *http.Client {
	base := c.httpClient()
	client := &http.Client{
		Timeout:   0,
		Transport: base.Transport,
	}
	if !followRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}
