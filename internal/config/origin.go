// Package config resolves the API origin. config.toml never holds a token.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Dir is $XDG_CONFIG_HOME/genos, or ~/.config/genos when the variable is empty.
func Dir(xdgConfigHome, home string) string {
	if strings.TrimSpace(xdgConfigHome) != "" {
		return filepath.Join(xdgConfigHome, "genos")
	}
	return filepath.Join(home, ".config", "genos")
}

// ResolveOrigin returns GENOS_HOST when it is non-empty, otherwise currentHost
// from configPath.
func ResolveOrigin(hostEnv, configPath string) (string, error) {
	if strings.TrimSpace(hostEnv) != "" {
		return ValidateOrigin(hostEnv)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("set GENOS_HOST or currentHost in config.toml")
		}
		return "", err
	}
	host, ok, err := ParseCurrentHost(string(data))
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(host) == "" {
		return "", errors.New("set GENOS_HOST or currentHost in config.toml")
	}
	return ValidateOrigin(host)
}

// ParseCurrentHost reads the currentHost assignment. The last assignment wins.
func ParseCurrentHost(toml string) (string, bool, error) {
	var (
		found string
		ok    bool
	)
	for lineNo, line := range strings.Split(toml, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, hasEq := strings.Cut(line, "=")
		if !hasEq {
			continue
		}
		if strings.TrimSpace(key) != "currentHost" {
			continue
		}
		parsed, err := parseTOMLString(strings.TrimSpace(value))
		if err != nil {
			return "", false, fmt.Errorf("config.toml:%d: %w", lineNo+1, err)
		}
		found = parsed
		ok = true
	}
	return found, ok, nil
}

func parseTOMLString(value string) (string, error) {
	if value == "" {
		return "", errors.New("currentHost is empty")
	}
	if value[0] == '"' || value[0] == '\'' {
		quote := value[0]
		rest := value[1:]
		end := strings.IndexByte(rest, quote)
		if end < 0 {
			return "", errors.New("currentHost string is not closed")
		}
		return rest[:end], nil
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value, nil
}

// ValidateOrigin canonicalizes an API origin. http is accepted only for
// localhost, 127.0.0.1, and genos.localhost.
func ValidateOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("origin is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid origin: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("origin %q needs a scheme and host", raw)
	}
	if parsed.User != nil {
		return "", errors.New("origin must not include user info")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("origin must not include a path, query, or fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	switch scheme {
	case "https":
		if port == "443" {
			port = ""
		}
	case "http":
		if !loopbackHost(host) {
			return "", fmt.Errorf("http is only allowed for loopback hosts (localhost, 127.0.0.1, genos.localhost); %s requires https", host)
		}
		if port == "80" {
			port = ""
		}
	default:
		return "", fmt.Errorf("origin scheme %q is not supported", scheme)
	}
	if port != "" {
		return scheme + "://" + host + ":" + port, nil
	}
	return scheme + "://" + host, nil
}

func loopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "genos.localhost":
		return true
	default:
		return false
	}
}
