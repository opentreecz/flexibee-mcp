// Package config loads and validates FlexiBee MCP server configuration from
// environment variables.  It never logs secret values.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultTimeoutMS = 30_000
	minTimeoutMS     = 1_000
	defaultPageSize  = 50
	maxPageSize      = 200
)

// Config holds validated runtime configuration for the FlexiBee MCP server.
// The Password field must never appear in log output.
type Config struct {
	BaseURL   string
	Company   string // optional default company
	Username  string
	Password  string // secret – never log
	TimeoutMS int
	PageSize  int
	CAFile    string
	AllowHTTP bool
}

// Load reads environment variables, validates them, and returns a *Config.
// It returns a descriptive error if any required value is missing, mutually
// exclusive values are both set, or a value is out of range.
func Load() (*Config, error) {
	rawURL := os.Getenv("FLEXIBEE_URL")
	company := os.Getenv("FLEXIBEE_COMPANY")
	username := os.Getenv("FLEXIBEE_USERNAME")
	password := os.Getenv("FLEXIBEE_PASSWORD")
	passwordFile := os.Getenv("FLEXIBEE_PASSWORD_FILE")
	timeoutStr := os.Getenv("FLEXIBEE_TIMEOUT_MS")
	pageSizeStr := os.Getenv("FLEXIBEE_PAGE_SIZE")
	caFile := os.Getenv("FLEXIBEE_CA_FILE")
	allowHTTPStr := os.Getenv("FLEXIBEE_ALLOW_HTTP")

	// ── Required fields ──────────────────────────────────────────────────────

	if rawURL == "" {
		return nil, errors.New("FLEXIBEE_URL is required")
	}
	if username == "" {
		return nil, errors.New("FLEXIBEE_USERNAME is required")
	}

	// ── Mutual exclusion: password vs. password_file ──────────────────────────

	if password != "" && passwordFile != "" {
		return nil, errors.New("FLEXIBEE_PASSWORD and FLEXIBEE_PASSWORD_FILE are mutually exclusive")
	}
	if password == "" && passwordFile == "" {
		return nil, errors.New("one of FLEXIBEE_PASSWORD or FLEXIBEE_PASSWORD_FILE is required")
	}

	// ── Resolve password from file ────────────────────────────────────────────

	if passwordFile != "" {
		raw, err := os.ReadFile(passwordFile)
		if err != nil {
			return nil, fmt.Errorf("reading FLEXIBEE_PASSWORD_FILE %q: %w", passwordFile, err)
		}
		password = strings.TrimRight(string(raw), " \t\r\n")
		if password == "" {
			return nil, fmt.Errorf("FLEXIBEE_PASSWORD_FILE %q is empty after trimming whitespace", passwordFile)
		}
	}

	// ── URL validation ────────────────────────────────────────────────────────

	allowHTTP := strings.EqualFold(allowHTTPStr, "true")

	baseURL, err := validateURL(rawURL, allowHTTP)
	if err != nil {
		return nil, err
	}

	// ── Optional integer fields ───────────────────────────────────────────────

	timeoutMS, err := parseIntEnv("FLEXIBEE_TIMEOUT_MS", timeoutStr, defaultTimeoutMS, minTimeoutMS, -1)
	if err != nil {
		return nil, err
	}

	pageSize, err := parseIntEnv("FLEXIBEE_PAGE_SIZE", pageSizeStr, defaultPageSize, 1, maxPageSize)
	if err != nil {
		return nil, err
	}

	return &Config{
		BaseURL:   baseURL,
		Company:   company,
		Username:  username,
		Password:  password,
		TimeoutMS: timeoutMS,
		PageSize:  pageSize,
		CAFile:    caFile,
		AllowHTTP: allowHTTP,
	}, nil
}

// validateURL parses rawURL and enforces the HTTP security policy.
// Path prefixes configured in the URL are preserved unchanged.
func validateURL(rawURL string, allowHTTP bool) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("FLEXIBEE_URL %q is not a valid URL: %w", rawURL, err)
	}

	switch u.Scheme {
	case "https":
		// Always allowed.
	case "http":
		if !allowHTTP {
			return "", fmt.Errorf(
				"FLEXIBEE_URL uses plain http:// – set FLEXIBEE_ALLOW_HTTP=true to permit it for loopback hosts",
			)
		}
		if !isLoopback(u.Hostname()) {
			return "", fmt.Errorf(
				"FLEXIBEE_URL uses plain http:// with non-loopback host %q; only loopback addresses are permitted with FLEXIBEE_ALLOW_HTTP=true",
				u.Hostname(),
			)
		}
	default:
		return "", fmt.Errorf("FLEXIBEE_URL scheme %q is not supported; use https or http", u.Scheme)
	}

	if u.Host == "" {
		return "", fmt.Errorf("FLEXIBEE_URL %q has no host", rawURL)
	}

	// Return the URL as-is, preserving any path prefix, but strip a trailing
	// slash for consistency so callers can simply append "/c/company/...".
	result := strings.TrimRight(rawURL, "/")
	return result, nil
}

// isLoopback reports whether host is a loopback address or hostname.
func isLoopback(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// parseIntEnv parses an integer from a string environment value.
// If the raw string is empty the default is returned.
// min is enforced unconditionally; max is only enforced when max >= 0.
func parseIntEnv(name, raw string, def, min, max int) (int, error) {
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", name, raw)
	}
	if v < min {
		return 0, fmt.Errorf("%s must be at least %d, got %d", name, min, v)
	}
	if max >= 0 && v > max {
		return 0, fmt.Errorf("%s must be at most %d, got %d", name, max, v)
	}
	return v, nil
}
