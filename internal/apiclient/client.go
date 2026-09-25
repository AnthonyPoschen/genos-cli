// Package apiclient is the Genos HTTP API client used by the CLI.
package apiclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/config"
)

const userAgent = "genos-cli"

// ErrNoInteractive means the server has no interactive runtime channel.
var ErrNoInteractive = errors.New("no interactive runtime channel")

// Client calls one Genos origin with an optional bearer token.
type Client struct {
	Origin string
	Token  string
	HTTP   *http.Client
}

// New returns a client for origin. token may be empty for device login.
func New(origin, token string) *Client {
	return &Client{Origin: origin, Token: token}
}

// Server is the customer server object returned by the API.
type Server struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Game           Game     `json:"game"`
	Status         string   `json:"status"`
	NotableUpdates []string `json:"notableUpdates"`
	Metrics        *Metrics `json:"metrics"`
}

// Game is the nested game summary.
type Game struct {
	Name string `json:"name"`
}

// Metrics holds the fields the CLI uses for confirmation.
type Metrics struct {
	PlayerCount *int64 `json:"playerCount"`
}

// APIError is a non-2xx Genos response.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	switch {
	case e.Code != "" && e.Message != "":
		return e.Code + ": " + e.Message
	case e.Message != "":
		return e.Message
	case e.Code != "":
		return e.Code
	default:
		return fmt.Sprintf("request failed with status %d", e.Status)
	}
}

// ListServers calls GET /api/v1/servers.
func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/servers", nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Servers []Server `json:"servers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("servers response: %w", err)
	}
	return payload.Servers, nil
}

// GetServer calls GET /api/v1/servers/{id}. The body may be the server object
// or {"server": <object>}.
func (c *Client) GetServer(ctx context.Context, id string) (Server, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/servers/"+url.PathEscape(id), nil, false)
	if err != nil {
		return Server{}, err
	}
	if status < 200 || status >= 300 {
		return Server{}, apiError(status, data)
	}
	server, err := decodeServer(data)
	if err != nil {
		return Server{}, err
	}
	if server.ID == "" {
		return Server{}, errors.New("server response did not include an id")
	}
	return server, nil
}

// RequestAction calls POST /api/v1/servers/{id}/actions.
func (c *Client) RequestAction(ctx context.Context, id, action string, confirmUnsaved bool) error {
	body := struct {
		Type                       string `json:"type"`
		ConfirmUnsavedProgressLoss bool   `json:"confirmUnsavedProgressLoss"`
	}{
		Type:                       action,
		ConfirmUnsavedProgressLoss: confirmUnsaved,
	}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/servers/"+url.PathEscape(id)+"/actions", body, true)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return apiError(status, data)
	}
	return nil
}

// Console posts text to the first interactive runtime channel and returns
// the response field.
func (c *Client) Console(ctx context.Context, id, text string) (string, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/servers/"+url.PathEscape(id)+"/runtime-channels", nil, false)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", apiError(status, data)
	}
	var channels struct {
		Channels []struct {
			ID          string `json:"id"`
			Interaction string `json:"interaction"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(data, &channels); err != nil {
		return "", fmt.Errorf("runtime channels: %w", err)
	}
	channelID := ""
	for _, channel := range channels.Channels {
		if channel.Interaction == "interactive" && channel.ID != "" {
			channelID = channel.ID
			break
		}
	}
	if channelID == "" {
		return "", ErrNoInteractive
	}
	data, status, err = c.do(ctx, http.MethodPost, "/api/v1/servers/"+url.PathEscape(id)+"/runtime-channels/"+url.PathEscape(channelID)+"/commands", map[string]string{"text": text}, false)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", apiError(status, data)
	}
	var payload struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("console response: %w", err)
	}
	return payload.Response, nil
}

// DeviceCode is the start of a device login.
type DeviceCode struct {
	DeviceCode       string
	UserCode         string
	VerificationPath string
	ExpiresIn        int
	Interval         int
	ExpiresAt        time.Time
}

// RequestDeviceCode calls POST /api/v1/auth/device/codes.
func (c *Client) RequestDeviceCode(ctx context.Context, machineName string) (DeviceCode, error) {
	if strings.TrimSpace(machineName) == "" {
		machineName = "unknown"
	}
	body := map[string]string{
		"clientName":  "genos-cli",
		"machineName": machineName,
		"host":        c.Origin,
	}
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/auth/device/codes", body, false)
	if err != nil {
		return DeviceCode{}, err
	}
	if status == http.StatusNotFound {
		return DeviceCode{}, errors.New("device login requires an upcoming Genos API; use genos auth token (PAT) instead")
	}
	if status < 200 || status >= 300 {
		return DeviceCode{}, apiError(status, data)
	}
	return decodeDeviceCode(data)
}

// PollDeviceToken calls POST /api/v1/auth/device/tokens until a token, a
// terminal error, or deadline. sleep may be nil.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string, interval time.Duration, deadline time.Time, sleep func(time.Duration)) (string, error) {
	if sleep == nil {
		sleep = time.Sleep
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	for {
		if !time.Now().Before(deadline) {
			return "", errors.New("device login expired")
		}
		token, pending, slow, err := c.pollDeviceOnce(ctx, deviceCode)
		if err != nil {
			return "", err
		}
		if token != "" {
			return token, nil
		}
		if !pending {
			return "", errors.New("device login failed")
		}
		if slow {
			interval += 5 * time.Second
			if interval > time.Minute {
				interval = time.Minute
			}
		}
		wait := interval
		if remain := time.Until(deadline); wait > remain {
			wait = remain
		}
		if wait > 0 {
			sleep(wait)
		}
	}
}

func (c *Client) pollDeviceOnce(ctx context.Context, deviceCode string) (token string, pending bool, slow bool, err error) {
	data, status, err := c.do(ctx, http.MethodPost, "/api/v1/auth/device/tokens", map[string]string{"deviceCode": deviceCode}, false)
	if err != nil {
		return "", false, false, err
	}
	token, code, message, err := parsePoll(data)
	if err != nil {
		return "", false, false, err
	}
	if token != "" && status >= 200 && status < 300 {
		return token, false, false, nil
	}
	switch code {
	case "authorization_pending", "pending":
		return "", true, false, nil
	case "slow_down":
		return "", true, true, nil
	case "access_denied", "expired", "expired_token":
		if message == "" {
			message = code
		}
		return "", false, false, errors.New(message)
	}
	if status == http.StatusTooManyRequests {
		return "", true, true, nil
	}
	if message == "" {
		message = fmt.Sprintf("device login failed with status %d", status)
	}
	return "", false, false, errors.New(message)
}

// ApprovalURL is origin joined with the verification path from the API.
// An absolute URL is used unchanged when it is the same origin. The access
// token is not added.
func ApprovalURL(origin, verificationPath string) (string, error) {
	verificationPath = strings.TrimSpace(verificationPath)
	if verificationPath == "" {
		return "", errors.New("verification path is empty")
	}
	if strings.Contains(verificationPath, "://") {
		parsed, err := url.Parse(verificationPath)
		if err != nil {
			return "", err
		}
		if parsed.Host == "" {
			return "", errors.New("verification URL is invalid")
		}
		got, err := config.ValidateOrigin(parsed.Scheme + "://" + parsed.Host)
		if err != nil {
			return "", err
		}
		if got != origin {
			return "", fmt.Errorf("verification URL origin %s does not match %s", got, origin)
		}
		return verificationPath, nil
	}
	if strings.ContainsAny(verificationPath, " \t\r\n") {
		return "", errors.New("verification path is invalid")
	}
	if !strings.HasPrefix(verificationPath, "/") {
		verificationPath = "/" + verificationPath
	}
	return strings.TrimRight(origin, "/") + verificationPath, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, idempotency bool) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if idempotency {
		key, err := newUUID()
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func (c *Client) endpoint(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(c.Origin, "/") + path
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTP
}

var defaultHTTP = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func decodeServer(data []byte) (Server, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return Server{}, fmt.Errorf("server response: %w", err)
	}
	if raw, ok := probe["server"]; ok && len(raw) > 0 && string(raw) != "null" {
		// A bare server object has no "server" member. The get endpoint may
		// wrap the same object.
		if _, bare := probe["id"]; !bare {
			var server Server
			if err := json.Unmarshal(raw, &server); err != nil {
				return Server{}, fmt.Errorf("server response: %w", err)
			}
			return server, nil
		}
	}
	var server Server
	if err := json.Unmarshal(data, &server); err != nil {
		return Server{}, fmt.Errorf("server response: %w", err)
	}
	return server, nil
}

func decodeDeviceCode(data []byte) (DeviceCode, error) {
	var body struct {
		DeviceCode       string `json:"deviceCode"`
		UserCode         string `json:"userCode"`
		VerificationPath string `json:"verificationPath"`
		ExpiresIn        int    `json:"expiresIn"`
		Interval         int    `json:"interval"`
		ExpiresAt        string `json:"expiresAt"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return DeviceCode{}, fmt.Errorf("device code response: %w", err)
	}
	if body.DeviceCode == "" || body.UserCode == "" || body.VerificationPath == "" {
		return DeviceCode{}, errors.New("device login response missing deviceCode, userCode, or verificationPath")
	}
	code := DeviceCode{
		DeviceCode:       body.DeviceCode,
		UserCode:         body.UserCode,
		VerificationPath: body.VerificationPath,
		ExpiresIn:        body.ExpiresIn,
		Interval:         body.Interval,
	}
	if body.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, body.ExpiresAt)
		if err != nil {
			return DeviceCode{}, fmt.Errorf("invalid expiresAt: %w", err)
		}
		code.ExpiresAt = parsed
	}
	return code, nil
}

func parsePoll(data []byte) (token, code, message string, err error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return "", "", "", nil
	}
	var body struct {
		AccessToken string          `json:"accessToken"`
		Token       string          `json:"token"`
		Error       json.RawMessage `json:"error"`
		Code        string          `json:"code"`
		Message     string          `json:"message"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", "", "", fmt.Errorf("device token response: %w", err)
	}
	errCode, errMessage := decodeErrorValue(body.Error)
	if errCode == "" {
		errCode = body.Code
	}
	if errMessage == "" {
		errMessage = body.Message
	}
	if body.AccessToken != "" {
		return body.AccessToken, errCode, errMessage, nil
	}
	return body.Token, errCode, errMessage, nil
}

func apiError(status int, data []byte) error {
	_, code, message, err := parsePoll(data)
	if err != nil || (code == "" && message == "") {
		message = strings.TrimSpace(string(data))
		if len(message) > 200 {
			message = message[:200]
		}
	}
	return &APIError{Status: status, Code: code, Message: message}
}

func decodeErrorValue(raw json.RawMessage) (code, message string) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, ""
	}
	var asObject struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject.Code, asObject.Message
	}
	return "", ""
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

// SavedSetup is one profile in a SetupChooser.
type SavedSetup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Game Game   `json:"game"`
}

// SavedSetupCapacity is the profile library capacity summary.
type SavedSetupCapacity struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

// SetupGame is a game that can still create a profile.
type SetupGame struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// SetupChooser is GET /api/v1/servers/{id}/setups.
type SetupChooser struct {
	Setups          []SavedSetup       `json:"setups"`
	SelectedSetupID string             `json:"selectedSetupID"`
	Capacity        SavedSetupCapacity `json:"capacity"`
	CreatableGames  []SetupGame        `json:"creatableGames"`
}

// ListSetups calls GET /api/v1/servers/{id}/setups.
func (c *Client) ListSetups(ctx context.Context, serverID string) (SetupChooser, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/servers/"+url.PathEscape(serverID)+"/setups", nil, false)
	if err != nil {
		return SetupChooser{}, err
	}
	if status < 200 || status >= 300 {
		return SetupChooser{}, apiError(status, data)
	}
	var chooser SetupChooser
	if err := json.Unmarshal(data, &chooser); err != nil {
		return SetupChooser{}, fmt.Errorf("setups response: %w", err)
	}
	if chooser.Setups == nil {
		chooser.Setups = []SavedSetup{}
	}
	return chooser, nil
}

// SelectSetup calls PUT /api/v1/servers/{id}/selected-setup.
// Idempotency-Key is not sent (matches Omarchy / dashboard).
func (c *Client) SelectSetup(ctx context.Context, serverID, setupID, expectedSelectedSetupID string) error {
	body := struct {
		SetupID                 string `json:"setupID"`
		ExpectedSelectedSetupID string `json:"expectedSelectedSetupID"`
	}{
		SetupID:                 setupID,
		ExpectedSelectedSetupID: expectedSelectedSetupID,
	}
	data, status, err := c.do(ctx, http.MethodPut, "/api/v1/servers/"+url.PathEscape(serverID)+"/selected-setup", body, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return apiError(status, data)
	}
	return nil
}

// UnloadSetup calls DELETE /api/v1/servers/{id}/selected-setup.
// Idempotency-Key is not sent (matches Omarchy / dashboard).
func (c *Client) UnloadSetup(ctx context.Context, serverID, expectedSelectedSetupID string) error {
	body := struct {
		ExpectedSelectedSetupID string `json:"expectedSelectedSetupID"`
	}{
		ExpectedSelectedSetupID: expectedSelectedSetupID,
	}
	data, status, err := c.do(ctx, http.MethodDelete, "/api/v1/servers/"+url.PathEscape(serverID)+"/selected-setup", body, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return apiError(status, data)
	}
	return nil
}

// Configuration is the selected setup configuration for a server.
type Configuration struct {
	SetupID        string               `json:"setupID"`
	Name           string               `json:"name,omitempty"`
	GameID         string               `json:"gameID"`
	GameName       string               `json:"gameName,omitempty"`
	ServerID       string               `json:"serverID,omitempty"`
	ServerName     string               `json:"serverName,omitempty"`
	Selected       bool                 `json:"selected,omitempty"`
	Version        string               `json:"version"`
	Values         json.RawMessage      `json:"values"`
	Editable       bool                 `json:"editable"`
	ReadOnlyReason string               `json:"readOnlyReason,omitempty"`
	UpdatedAt      time.Time            `json:"updatedAt"`
	CreatedAt      time.Time            `json:"createdAt,omitempty"`
	Secrets        ConfigurationSecrets `json:"secrets"`
}

// ConfigurationSecrets is the secret status summary on a configuration.
type ConfigurationSecrets struct {
	Version    string `json:"version"`
	Configured bool   `json:"configured"`
	Pending    bool   `json:"pending"`
}

// GetConfiguration calls GET /api/v1/servers/{id}/configuration.
// Idempotency-Key is not sent (matches dashboard).
func (c *Client) GetConfiguration(ctx context.Context, serverID string) (Configuration, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/servers/"+url.PathEscape(serverID)+"/configuration", nil, false)
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
		return Configuration{}, fmt.Errorf("configuration response: %w", err)
	}
	return payload.Configuration, nil
}

// PutConfiguration calls PUT /api/v1/servers/{id}/configuration.
// body must already include expectedSetupID, expectedUpdatedAt, version, and values.
// Idempotency-Key is not sent (matches dashboard).
func (c *Client) PutConfiguration(ctx context.Context, serverID string, body any) (Configuration, error) {
	data, status, err := c.do(ctx, http.MethodPut, "/api/v1/servers/"+url.PathEscape(serverID)+"/configuration", body, false)
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
		return Configuration{}, fmt.Errorf("configuration response: %w", err)
	}
	return payload.Configuration, nil
}

// GetManagementSchema calls GET /api/v1/games/{gameID}/management-schema.
// The route is public; auth is optional. Idempotency-Key is not sent.
func (c *Client) GetManagementSchema(ctx context.Context, gameID string) (json.RawMessage, error) {
	data, status, err := c.do(ctx, http.MethodGet, "/api/v1/games/"+url.PathEscape(gameID)+"/management-schema", nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	if !json.Valid(data) {
		return nil, errors.New("management schema response is not valid JSON")
	}
	return json.RawMessage(data), nil
}

// SetupModState is the selected setup mod projection for a server.
type SetupModState struct {
	SetupID               string          `json:"setupID"`
	GameID                string          `json:"gameID"`
	Editable              bool            `json:"editable"`
	ReadOnlyReason        string          `json:"readOnlyReason,omitempty"`
	CredentialsConfigured bool            `json:"credentialsConfigured"`
	Staged                *SetupMod       `json:"staged,omitempty"`
	Enabled               *SetupMod       `json:"enabled,omitempty"`
	Installed             json.RawMessage `json:"installed,omitempty"`
	Available             json.RawMessage `json:"available,omitempty"`
	Update                json.RawMessage `json:"update,omitempty"`
	Issues                json.RawMessage `json:"issues,omitempty"`
	Collection            json.RawMessage `json:"collection,omitempty"`
}

// SetupMod is one staged or enabled mod entry.
type SetupMod struct {
	Operation     string    `json:"operation,omitempty"`
	ProviderID    string    `json:"providerID"`
	ProviderModID string    `json:"providerModID"`
	Name          string    `json:"name,omitempty"`
	Title         string    `json:"title,omitempty"`
	Version       string    `json:"version,omitempty"`
	GameVersion   string    `json:"gameVersion,omitempty"`
	Dependencies  []string  `json:"dependencies,omitempty"`
	LoadIDs       []string  `json:"loadIDs,omitempty"`
	StageID       string    `json:"stageID,omitempty"`
	StagedAt      time.Time `json:"stagedAt,omitempty"`
	AppliedAt     time.Time `json:"appliedAt,omitempty"`
}

// ModCatalogQuery is GET /api/v1/servers/{id}/mods/catalog.
type ModCatalogQuery struct {
	Query    string
	Category string
	Sort     string
	Page     int
	PageSize int
}

func modsPath(serverID, suffix string) string {
	base := "/api/v1/servers/" + url.PathEscape(serverID) + "/mods"
	if suffix == "" {
		return base
	}
	return base + suffix
}

func decodeModsPayload(data []byte) (SetupModState, error) {
	var payload struct {
		Mods SetupModState `json:"mods"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return SetupModState{}, fmt.Errorf("mods response: %w", err)
	}
	return payload.Mods, nil
}

// GetMods calls GET /api/v1/servers/{id}/mods.
// Idempotency-Key is not sent (matches dashboard).
func (c *Client) GetMods(ctx context.Context, serverID string) (SetupModState, error) {
	data, status, err := c.do(ctx, http.MethodGet, modsPath(serverID, ""), nil, false)
	if err != nil {
		return SetupModState{}, err
	}
	if status < 200 || status >= 300 {
		return SetupModState{}, apiError(status, data)
	}
	return decodeModsPayload(data)
}

// SearchModCatalog calls GET /api/v1/servers/{id}/mods/catalog.
// Returns the inner catalog JSON object as returned by the API.
func (c *Client) SearchModCatalog(ctx context.Context, serverID string, query ModCatalogQuery) (json.RawMessage, error) {
	values := url.Values{}
	if query.Query != "" {
		values.Set("q", query.Query)
	}
	if query.Category != "" {
		values.Set("category", query.Category)
	}
	if query.Sort != "" {
		values.Set("sort", query.Sort)
	}
	if query.Page > 0 {
		values.Set("page", strconv.Itoa(query.Page))
	}
	if query.PageSize > 0 {
		values.Set("pageSize", strconv.Itoa(query.PageSize))
	}
	path := modsPath(serverID, "/catalog")
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	data, status, err := c.do(ctx, http.MethodGet, path, nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Catalog json.RawMessage `json:"catalog"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("mod catalog response: %w", err)
	}
	if len(payload.Catalog) == 0 || string(payload.Catalog) == "null" {
		return nil, errors.New("mod catalog response missing catalog")
	}
	return payload.Catalog, nil
}

// InspectModCatalog calls GET /api/v1/servers/{id}/mods/catalog/{modID}.
// Returns the inner mod JSON object as returned by the API.
func (c *Client) InspectModCatalog(ctx context.Context, serverID, providerModID string) (json.RawMessage, error) {
	data, status, err := c.do(ctx, http.MethodGet, modsPath(serverID, "/catalog/"+url.PathEscape(providerModID)), nil, false)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, apiError(status, data)
	}
	var payload struct {
		Mod json.RawMessage `json:"mod"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("mod catalog detail response: %w", err)
	}
	if len(payload.Mod) == 0 || string(payload.Mod) == "null" {
		return nil, errors.New("mod catalog detail response missing mod")
	}
	return payload.Mod, nil
}

// SetModCredentials calls PUT /api/v1/servers/{id}/mods/credentials.
func (c *Client) SetModCredentials(ctx context.Context, serverID, expectedSetupID, username, token string) (SetupModState, error) {
	body := struct {
		ExpectedSetupID string `json:"expectedSetupID"`
		Username        string `json:"username"`
		Token           string `json:"token"`
	}{
		ExpectedSetupID: expectedSetupID,
		Username:        username,
		Token:           token,
	}
	data, status, err := c.do(ctx, http.MethodPut, modsPath(serverID, "/credentials"), body, false)
	if err != nil {
		return SetupModState{}, err
	}
	if status < 200 || status >= 300 {
		return SetupModState{}, apiError(status, data)
	}
	return decodeModsPayload(data)
}

// ClearModCredentials calls DELETE /api/v1/servers/{id}/mods/credentials.
func (c *Client) ClearModCredentials(ctx context.Context, serverID, expectedSetupID string) (SetupModState, error) {
	body := struct {
		ExpectedSetupID string `json:"expectedSetupID"`
	}{
		ExpectedSetupID: expectedSetupID,
	}
	data, status, err := c.do(ctx, http.MethodDelete, modsPath(serverID, "/credentials"), body, false)
	if err != nil {
		return SetupModState{}, err
	}
	if status < 200 || status >= 300 {
		return SetupModState{}, apiError(status, data)
	}
	return decodeModsPayload(data)
}

// StageMod calls PUT /api/v1/servers/{id}/mods/staged-selection.
func (c *Client) StageMod(ctx context.Context, serverID, expectedSetupID, providerID, providerModID string) (SetupModState, error) {
	body := struct {
		ExpectedSetupID string `json:"expectedSetupID"`
		ProviderID      string `json:"providerID"`
		ProviderModID   string `json:"providerModID"`
	}{
		ExpectedSetupID: expectedSetupID,
		ProviderID:      providerID,
		ProviderModID:   providerModID,
	}
	data, status, err := c.do(ctx, http.MethodPut, modsPath(serverID, "/staged-selection"), body, false)
	if err != nil {
		return SetupModState{}, err
	}
	if status < 200 || status >= 300 {
		return SetupModState{}, apiError(status, data)
	}
	return decodeModsPayload(data)
}

// UnstageMod calls POST /api/v1/servers/{id}/mods/staged-removal.
// This stages removal of the currently enabled mod (not discard of a staged selection).
func (c *Client) UnstageMod(ctx context.Context, serverID, expectedSetupID string) (SetupModState, error) {
	body := struct {
		ExpectedSetupID string `json:"expectedSetupID"`
	}{
		ExpectedSetupID: expectedSetupID,
	}
	data, status, err := c.do(ctx, http.MethodPost, modsPath(serverID, "/staged-removal"), body, false)
	if err != nil {
		return SetupModState{}, err
	}
	if status < 200 || status >= 300 {
		return SetupModState{}, apiError(status, data)
	}
	return decodeModsPayload(data)
}

// DiscardMod calls POST /api/v1/servers/{id}/mods/discard.
func (c *Client) DiscardMod(ctx context.Context, serverID, expectedSetupID string) (SetupModState, error) {
	body := struct {
		ExpectedSetupID string `json:"expectedSetupID"`
	}{
		ExpectedSetupID: expectedSetupID,
	}
	data, status, err := c.do(ctx, http.MethodPost, modsPath(serverID, "/discard"), body, false)
	if err != nil {
		return SetupModState{}, err
	}
	if status < 200 || status >= 300 {
		return SetupModState{}, apiError(status, data)
	}
	return decodeModsPayload(data)
}

// ApplyMod calls POST /api/v1/servers/{id}/mods/apply.
func (c *Client) ApplyMod(ctx context.Context, serverID, expectedSetupID, stageID string) (SetupModState, error) {
	body := struct {
		ExpectedSetupID string `json:"expectedSetupID"`
		StageID         string `json:"stageID"`
	}{
		ExpectedSetupID: expectedSetupID,
		StageID:         stageID,
	}
	data, status, err := c.do(ctx, http.MethodPost, modsPath(serverID, "/apply"), body, false)
	if err != nil {
		return SetupModState{}, err
	}
	if status < 200 || status >= 300 {
		return SetupModState{}, apiError(status, data)
	}
	return decodeModsPayload(data)
}
