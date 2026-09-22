// Package creds resolves and stores a customer token.
// GENOS_TOKEN is never written. local.env is never read.
package creds

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// SourceEnv is a GENOS_TOKEN value passed into ResolveToken.
	SourceEnv = "env"
	// SourceKeyring is the OS keyring item for the API origin.
	SourceKeyring = "keyring"
	// SourceFile is credentials.json.
	SourceFile = "file"

	// Service is the keyring item service name.
	Service = "genos"
)

// Keyring is the OS credential store. Get and Set key the item by API origin.
// The production store uses service "genos" and the username attribute equal
// to that origin.
type Keyring interface {
	Get(origin string) (string, error)
	Set(origin, secret string) error
}

// File is one credentials.json lookup. Path is the only file Read.
type File struct {
	Path   string
	Origin string
}

// Result is a resolved bearer token and where it came from.
type Result struct {
	Token  string
	Source string
}

// ModeError means credentials.json is group- or world-readable.
type ModeError struct {
	Path string
}

func (e *ModeError) Error() string {
	return fmt.Sprintf("%s is group- or world-readable; chmod 0600 %s", e.Path, e.Path)
}

// ErrNotFound means none of the credential sources had a token.
var ErrNotFound = errors.New("no token found")

// ErrLocalEnv refuses the operator env file. Customer tokens do not live there.
var ErrLocalEnv = errors.New("refusing to read local.env")

// KeyringItem is the service and account passed to the OS keyring.
func KeyringItem(origin string) (service, account string) {
	return Service, origin
}

// ResolveToken returns the first hit among env, keyring, and file.
// env is used only when it is non-empty. A failed keyring lookup falls
// through. A group- or world-readable file is an error and is not used.
func ResolveToken(env string, keyring Keyring, file File) (Result, error) {
	if env != "" {
		return Result{Token: env, Source: SourceEnv}, nil
	}
	if keyring != nil {
		secret, err := keyring.Get(file.Origin)
		if err == nil && secret != "" {
			return Result{Token: secret, Source: SourceKeyring}, nil
		}
	}
	if file.Path == "" {
		return Result{}, ErrNotFound
	}
	token, err := readFileToken(file.Path, file.Origin)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, ErrNotFound
		}
		return Result{}, err
	}
	if token == "" {
		return Result{}, ErrNotFound
	}
	return Result{Token: token, Source: SourceFile}, nil
}

// Save stores token for origin. The keyring is tried first. When it is missing
// or the write fails, credentials.json is written at mode 0600. A directory is
// created at 0700 only when it does not already exist.
func Save(origin, token string, keyring Keyring, path string) (string, error) {
	if origin == "" || token == "" {
		return "", errors.New("origin and token are required")
	}
	if keyring != nil {
		if err := keyring.Set(origin, token); err == nil {
			return SourceKeyring, nil
		}
	}
	if err := writeFileToken(path, origin, token); err != nil {
		return "", err
	}
	return SourceFile, nil
}

// Prefix is the first 16 characters of a token. It is never the full secret
// when the token is longer than that.
func Prefix(token string) string {
	runes := []rune(token)
	if len(runes) <= 16 {
		return token
	}
	return string(runes[:16])
}

func readFileToken(path, origin string) (string, error) {
	if err := refuseLocalEnv(path); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if err := refuseLocalEnvTarget(path); err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	if info.Mode().Perm()&0o044 != 0 {
		return "", &ModeError{Path: path}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hosts, err := decodeHosts(data)
	if err != nil {
		return "", err
	}
	host, ok := hosts[origin]
	if !ok {
		return "", ErrNotFound
	}
	return host.Token, nil
}

func refuseLocalEnv(path string) error {
	if strings.EqualFold(filepath.Base(path), "local.env") {
		return ErrLocalEnv
	}
	return nil
}

func refuseLocalEnvTarget(path string) error {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil
	}
	if strings.EqualFold(filepath.Base(target), "local.env") {
		return ErrLocalEnv
	}
	return nil
}

type hostEntry struct {
	Token string `json:"token"`
}

type fileDocument struct {
	Hosts map[string]hostEntry `json:"hosts"`
}

func decodeHosts(data []byte) (map[string]hostEntry, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]hostEntry{}, nil
	}
	var doc fileDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("credentials.json: %w", err)
	}
	if doc.Hosts == nil {
		doc.Hosts = map[string]hostEntry{}
	}
	return doc.Hosts, nil
}

func writeFileToken(path, origin, token string) error {
	if err := refuseLocalEnv(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return err
	}
	hosts := map[string]hostEntry{}
	if _, err := os.Stat(path); err == nil {
		if err := refuseLocalEnvTarget(path); err != nil {
			return err
		}
		existing, err := readHostsForUpdate(path)
		if err != nil {
			return err
		}
		hosts = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	hosts[origin] = hostEntry{Token: token}
	payload, err := json.MarshalIndent(fileDocument{Hosts: hosts}, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return writeAtomic(path, payload)
}

func readHostsForUpdate(path string) (map[string]hostEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o044 != 0 {
		return nil, &ModeError{Path: path}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeHosts(data)
}

func ensureDir(dir string) error {
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
		// An existing directory keeps its mode.
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

func writeAtomic(path string, payload []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	cleanup = false
	return os.Chmod(path, 0o600)
}
