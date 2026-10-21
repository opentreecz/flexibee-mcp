package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setenv sets multiple env vars and returns a cleanup function that restores
// their original values (including unsetting vars that were not previously set).
func setenv(t *testing.T, pairs map[string]string) {
	t.Helper()
	orig := make(map[string]string, len(pairs))
	wasSet := make(map[string]bool, len(pairs))
	for k := range pairs {
		v, ok := os.LookupEnv(k)
		orig[k] = v
		wasSet[k] = ok
	}
	for k, v := range pairs {
		if err := os.Setenv(k, v); err != nil {
			t.Fatalf("os.Setenv(%q): %v", k, err)
		}
	}
	t.Cleanup(func() {
		for k := range pairs {
			if wasSet[k] {
				os.Setenv(k, orig[k]) //nolint:errcheck
			} else {
				os.Unsetenv(k) //nolint:errcheck
			}
		}
	})
}

// writeTempFile writes content to a temp file and returns its path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "secret-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return f.Name()
}

func TestLoad(t *testing.T) {
	// baseline valid env – every test case merges overrides on top of this
	const (
		validURL      = "https://flexibee.example.com:5434"
		validUser     = "admin"
		validPassword = "s3cr3t"
	)

	baseline := map[string]string{
		"FLEXIBEE_URL":           validURL,
		"FLEXIBEE_COMPANY":       "",
		"FLEXIBEE_USERNAME":      validUser,
		"FLEXIBEE_PASSWORD":      validPassword,
		"FLEXIBEE_PASSWORD_FILE": "",
		"FLEXIBEE_TIMEOUT_MS":    "",
		"FLEXIBEE_PAGE_SIZE":     "",
		"FLEXIBEE_CA_FILE":       "",
		"FLEXIBEE_ALLOW_HTTP":    "",
	}

	merge := func(base, over map[string]string) map[string]string {
		m := make(map[string]string, len(base)+len(over))
		for k, v := range base {
			m[k] = v
		}
		for k, v := range over {
			m[k] = v
		}
		return m
	}

	t.Run("valid config with password", func(t *testing.T) {
		setenv(t, baseline)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != validURL {
			t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, validURL)
		}
		if cfg.Username != validUser {
			t.Errorf("Username = %q, want %q", cfg.Username, validUser)
		}
		if cfg.Password != validPassword {
			t.Errorf("Password mismatch (not logging value)")
		}
		if cfg.TimeoutMS != defaultTimeoutMS {
			t.Errorf("TimeoutMS = %d, want %d", cfg.TimeoutMS, defaultTimeoutMS)
		}
		if cfg.PageSize != defaultPageSize {
			t.Errorf("PageSize = %d, want %d", cfg.PageSize, defaultPageSize)
		}
	})

	t.Run("valid config with password_file", func(t *testing.T) {
		pf := writeTempFile(t, "filepassword")
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_PASSWORD":      "",
			"FLEXIBEE_PASSWORD_FILE": pf,
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Password != "filepassword" {
			t.Errorf("Password from file mismatch (not logging value)")
		}
	})

	t.Run("error: both password and password_file set", func(t *testing.T) {
		pf := writeTempFile(t, "filepassword")
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_PASSWORD":      "direct",
			"FLEXIBEE_PASSWORD_FILE": pf,
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for mutually exclusive credentials, got nil")
		}
	})

	t.Run("error: no URL", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_URL": "",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for missing URL, got nil")
		}
	})

	t.Run("error: no username", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_USERNAME": "",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for missing username, got nil")
		}
	})

	t.Run("error: no credentials", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_PASSWORD":      "",
			"FLEXIBEE_PASSWORD_FILE": "",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for missing credentials, got nil")
		}
	})

	t.Run("secret file with trailing newline is trimmed", func(t *testing.T) {
		pf := writeTempFile(t, "trimme\n")
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_PASSWORD":      "",
			"FLEXIBEE_PASSWORD_FILE": pf,
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Password != "trimme" {
			t.Errorf("expected trailing newline to be trimmed (not logging value)")
		}
	})

	t.Run("URL with path prefix is preserved", func(t *testing.T) {
		urlWithPrefix := "https://proxy.example.com/flexibee/api"
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_URL": urlWithPrefix,
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != urlWithPrefix {
			t.Errorf("BaseURL = %q, want %q (path prefix must be preserved)", cfg.BaseURL, urlWithPrefix)
		}
	})

	t.Run("URL with trailing slash is stripped", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_URL": "https://flexibee.example.com:5434/",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != "https://flexibee.example.com:5434" {
			t.Errorf("BaseURL = %q, trailing slash should be stripped", cfg.BaseURL)
		}
	})

	t.Run("plain http to non-loopback without override is rejected", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_URL":        "http://flexibee.example.com:5434",
			"FLEXIBEE_ALLOW_HTTP": "",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for plain http to non-loopback host, got nil")
		}
	})

	t.Run("plain http to non-loopback with override is still rejected", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_URL":        "http://flexibee.example.com:5434",
			"FLEXIBEE_ALLOW_HTTP": "true",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for plain http to non-loopback host even with FLEXIBEE_ALLOW_HTTP=true")
		}
	})

	for _, loopback := range []string{"localhost", "127.0.0.1", "::1"} {
		loopback := loopback
		t.Run("plain http to loopback "+loopback+" with FLEXIBEE_ALLOW_HTTP=true is accepted", func(t *testing.T) {
			rawURL := "http://" + loopback + ":5434"
			if loopback == "::1" {
				rawURL = "http://[::1]:5434"
			}
			setenv(t, merge(baseline, map[string]string{
				"FLEXIBEE_URL":        rawURL,
				"FLEXIBEE_ALLOW_HTTP": "true",
			}))
			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error for loopback %q: %v", loopback, err)
			}
			if !cfg.AllowHTTP {
				t.Errorf("AllowHTTP should be true")
			}
		})
	}

	t.Run("timeout and page size parsing", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_TIMEOUT_MS": "5000",
			"FLEXIBEE_PAGE_SIZE":  "100",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TimeoutMS != 5000 {
			t.Errorf("TimeoutMS = %d, want 5000", cfg.TimeoutMS)
		}
		if cfg.PageSize != 100 {
			t.Errorf("PageSize = %d, want 100", cfg.PageSize)
		}
	})

	t.Run("timeout below minimum is rejected", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_TIMEOUT_MS": "500",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for timeout below minimum, got nil")
		}
	})

	t.Run("page size above maximum is rejected", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_PAGE_SIZE": "201",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for page size above maximum, got nil")
		}
	})

	t.Run("timeout default when unset", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_TIMEOUT_MS": "",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TimeoutMS != defaultTimeoutMS {
			t.Errorf("TimeoutMS = %d, want default %d", cfg.TimeoutMS, defaultTimeoutMS)
		}
	})

	t.Run("page size default when unset", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_PAGE_SIZE": "",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.PageSize != defaultPageSize {
			t.Errorf("PageSize = %d, want default %d", cfg.PageSize, defaultPageSize)
		}
	})

	t.Run("CA file path is stored but not read", func(t *testing.T) {
		// Point at a non-existent path – Load must not open the file.
		ghost := filepath.Join(t.TempDir(), "nonexistent-ca.pem")
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_CA_FILE": ghost,
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CAFile != ghost {
			t.Errorf("CAFile = %q, want %q", cfg.CAFile, ghost)
		}
	})

	t.Run("company is optional and passed through", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_COMPANY": "DEMO",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Company != "DEMO" {
			t.Errorf("Company = %q, want %q", cfg.Company, "DEMO")
		}
	})

	t.Run("auth mode defaults to auto", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_AUTH_MODE": "",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AuthMode != AuthModeAuto {
			t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, AuthModeAuto)
		}
	})

	t.Run("auth mode session is accepted", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_AUTH_MODE": "session",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AuthMode != AuthModeSession {
			t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, AuthModeSession)
		}
	})

	t.Run("auth mode api is accepted", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_AUTH_MODE": "api",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AuthMode != AuthModeAPI {
			t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, AuthModeAPI)
		}
	})

	t.Run("auth mode java is accepted", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_AUTH_MODE": "java",
		}))
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.AuthMode != AuthModeJava {
			t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, AuthModeJava)
		}
	})

	t.Run("error: invalid auth mode", func(t *testing.T) {
		setenv(t, merge(baseline, map[string]string{
			"FLEXIBEE_AUTH_MODE": "bogus",
		}))
		_, err := Load()
		if err == nil {
			t.Fatal("expected error for invalid auth mode, got nil")
		}
	})
}

// ── Profile file tests ────────────────────────────────────────────────────────

func TestLoadProfileFile(t *testing.T) {
	profileJSON := `{
		"profiles": {
			"production": {
				"url": "https://prod.example.com:5434",
				"username": "mcpbot",
				"password": "prodpass",
				"company": "prod-co",
				"auth_mode": "session",
				"timeout_ms": 10000
			},
			"staging": {
				"url": "https://staging.example.com:5434",
				"username": "mcpbot-dev",
				"password": "devpass",
				"company": "dev-co"
			}
		}
	}`
	cfgFile := writeTempFile(t, profileJSON)

	// Load "production" profile – all values from file.
	setenv(t, map[string]string{
		"FLEXIBEE_CONFIG_FILE": cfgFile,
		"FLEXIBEE_PROFILE":     "production",
		"FLEXIBEE_URL":         "",
		"FLEXIBEE_USERNAME":    "",
		"FLEXIBEE_PASSWORD":    "",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load (production profile): %v", err)
	}
	if !strings.Contains(cfg.BaseURL, "prod.example.com") {
		t.Errorf("BaseURL = %q, want prod.example.com", cfg.BaseURL)
	}
	if cfg.Company != "prod-co" {
		t.Errorf("Company = %q, want \"prod-co\"", cfg.Company)
	}
	if cfg.AuthMode != "session" {
		t.Errorf("AuthMode = %q, want \"session\"", cfg.AuthMode)
	}
	if cfg.TimeoutMS != 10000 {
		t.Errorf("TimeoutMS = %d, want 10000", cfg.TimeoutMS)
	}
}

func TestLoadProfileFileEnvOverridesProfile(t *testing.T) {
	profileJSON := `{
		"profiles": {
			"base": {
				"url": "https://base.example.com:5434",
				"username": "baseuser",
				"password": "basepass",
				"company": "base-co"
			}
		}
	}`
	cfgFile := writeTempFile(t, profileJSON)

	// Env var URL overrides the profile URL.
	setenv(t, map[string]string{
		"FLEXIBEE_CONFIG_FILE": cfgFile,
		"FLEXIBEE_PROFILE":     "base",
		"FLEXIBEE_URL":         "https://override.example.com:5434",
		"FLEXIBEE_USERNAME":    "",
		"FLEXIBEE_PASSWORD":    "",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load (override profile): %v", err)
	}
	if !strings.Contains(cfg.BaseURL, "override.example.com") {
		t.Errorf("BaseURL = %q, want override.example.com (env should override profile)", cfg.BaseURL)
	}
}

func TestLoadProfileFileUnknownProfile(t *testing.T) {
	profileJSON := `{"profiles":{"known":{"url":"https://x:5434","username":"u","password":"p"}}}`
	cfgFile := writeTempFile(t, profileJSON)

	setenv(t, map[string]string{
		"FLEXIBEE_CONFIG_FILE": cfgFile,
		"FLEXIBEE_PROFILE":     "no-such-profile",
		"FLEXIBEE_URL":         "",
		"FLEXIBEE_USERNAME":    "",
		"FLEXIBEE_PASSWORD":    "",
	})
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unknown profile, got nil")
	}
}
