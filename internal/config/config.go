// Package config loads and validates FlexiBee MCP server configuration from
// environment variables.  It never logs secret values.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// unmarshalJSON decodes JSON data into v. Package-level var so tests can swap it.
var unmarshalJSON = json.Unmarshal

const (
	defaultTimeoutMS = 30_000
	minTimeoutMS     = 1_000
	defaultPageSize  = 50
	maxPageSize      = 200

	// AuthModeAuto probes API first, falls back to session, then to Java.
	AuthModeAuto = "auto"
	// AuthModeAPI uses only HTTP Basic Auth REST (requires api-rw licence).
	AuthModeAPI = "api"
	// AuthModeSession uses v2 session auth mimicking a web browser (no licence needed).
	AuthModeSession = "session"
	// AuthModeJava uses the native Java flexibee-client as a subprocess.
	AuthModeJava = "java"
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

	// AuthMode controls which transport(s) are used.
	// Values: "auto" (default), "api", "session", "java".
	AuthMode string

	// JavaHome is the path to the Java installation directory (optional).
	// When empty, Go searches PATH and well-known locations for java.
	JavaHome string

	// FlexiBeeLib is the directory containing winstrom-connector-*.jar.
	// When empty, Go tries the standard installation paths.
	FlexiBeeLib string

	// SkipTLSVerify disables TLS certificate verification.
	// Only for development/testing – never use in production.
	SkipTLSVerify bool
}

// Load reads environment variables, validates them, and returns a *Config.
// It returns a descriptive error if any required value is missing, mutually
// exclusive values are both set, or a value is out of range.
//
// Load also supports FLEXIBEE_PROFILE for named profile selection from a
// FLEXIBEE_CONFIG_FILE JSON file.  When a profile file and name are both set,
// the profile values are used as defaults and individual env vars override them.
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
	authMode := os.Getenv("FLEXIBEE_AUTH_MODE")
	javaHome := os.Getenv("FLEXIBEE_JAVA_HOME")
	flexibeeLib := os.Getenv("FLEXIBEE_LIB")
	skipTLSStr := os.Getenv("FLEXIBEE_SKIP_TLS_VERIFY")

	// ── Profile file support ──────────────────────────────────────────────────
	// If FLEXIBEE_CONFIG_FILE and FLEXIBEE_PROFILE are set, load the named
	// profile from the JSON file and use its values as defaults for any
	// env var not already set.
	configFile := os.Getenv("FLEXIBEE_CONFIG_FILE")
	profileName := os.Getenv("FLEXIBEE_PROFILE")
	if configFile != "" && profileName != "" {
		p, err := loadProfile(configFile, profileName)
		if err != nil {
			return nil, fmt.Errorf("loading profile %q from %q: %w", profileName, configFile, err)
		}
		// Apply profile defaults (env vars take precedence).
		if rawURL == "" {
			rawURL = p.URL
		}
		if company == "" {
			company = p.Company
		}
		if username == "" {
			username = p.Username
		}
		if password == "" {
			password = p.Password
		}
		if passwordFile == "" {
			passwordFile = p.PasswordFile
		}
		if caFile == "" {
			caFile = p.CAFile
		}
		if authMode == "" {
			authMode = p.AuthMode
		}
		if timeoutStr == "" && p.TimeoutMS > 0 {
			timeoutStr = fmt.Sprintf("%d", p.TimeoutMS)
		}
	}

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

	// ── Auth mode ─────────────────────────────────────────────────────────────

	if authMode == "" {
		authMode = AuthModeAuto
	}
	switch authMode {
	case AuthModeAuto, AuthModeAPI, AuthModeSession, AuthModeJava:
		// valid
	default:
		return nil, fmt.Errorf("FLEXIBEE_AUTH_MODE %q is not valid; use auto, api, session, or java", authMode)
	}

	skipTLSVerify := strings.EqualFold(skipTLSStr, "true")

	return &Config{
		BaseURL:       baseURL,
		Company:       company,
		Username:      username,
		Password:      password,
		TimeoutMS:     timeoutMS,
		PageSize:      pageSize,
		CAFile:        caFile,
		AllowHTTP:     allowHTTP,
		AuthMode:      authMode,
		JavaHome:      javaHome,
		FlexiBeeLib:   flexibeeLib,
		SkipTLSVerify: skipTLSVerify,
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

// ── Profile file ──────────────────────────────────────────────────────────────

// ProfileEntry holds per-profile configuration values in a profiles JSON file.
// All fields are optional; individual env vars override them at load time.
type ProfileEntry struct {
	URL          string `json:"url"`
	Company      string `json:"company"`
	Username     string `json:"username"`
	Password     string `json:"password"`      // plaintext; prefer PasswordFile
	PasswordFile string `json:"password_file"` // path to a file with the password
	CAFile       string `json:"ca_file"`
	AuthMode     string `json:"auth_mode"`
	TimeoutMS    int    `json:"timeout_ms"`
}

// ProfileFile is the top-level structure of a FLEXIBEE_CONFIG_FILE JSON file.
//
// Example file:
//
//	{
//	  "profiles": {
//	    "production": {
//	      "url": "https://flexi.example.com:5434",
//	      "username": "mcpbot",
//	      "password_file": "/etc/flexibee/password",
//	      "company": "my-company"
//	    },
//	    "staging": {
//	      "url": "https://flexi-dev.example.com:5434",
//	      "username": "mcpbot-dev",
//	      "password": "devpassword",
//	      "company": "dev-company",
//	      "auth_mode": "session"
//	    }
//	  }
//	}
type ProfileFile struct {
	Profiles map[string]ProfileEntry `json:"profiles"`
}

// loadProfile reads profileName from the JSON config file at path.
func loadProfile(path, profileName string) (*ProfileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	var pf ProfileFile
	if err := unmarshalJSON(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	entry, ok := pf.Profiles[profileName]
	if !ok {
		names := make([]string, 0, len(pf.Profiles))
		for k := range pf.Profiles {
			names = append(names, k)
		}
		return nil, fmt.Errorf("profile %q not found; available: %v", profileName, names)
	}
	return &entry, nil
}
