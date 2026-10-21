// Package flexi – transport_session.go implements the v2 session-based transport.
//
// This transport authenticates via POST /v2/login-logout/login (form-urlencoded)
// which creates a ClientType.FLEXI session on the server.  The session uses the
// HTML (web) auth path that bypasses the rest-api-rw licence check entirely.
//
// Critical constraints (from binary reverse-engineering of FlexiBee 2026.6.2):
//  1. Use /v2/ prefix for ALL endpoints – creates ClientType.FLEXI session.
//  2. Login ONCE; reuse the session for all requests.
//  3. NEVER re-login while a session is active (obsoletes previous session).
//  4. CSRF token must be in ?token=<csrf> for write operations (PUT, DELETE, POST
//     against write endpoints).
//  5. Use a dedicated user account to avoid conflicting with browser sessions.
package flexi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/config"
)

// sessionUserAgent mimics the native ABRA Flexi desktop client header.
// Using "DESKTOP" client type to match what the Java client sends.
const sessionUserAgent = "WinStrom/2026.6.2 (DESKTOP)"

// sessionKeepAliveInterval is how often the keep-alive ping is sent.
const sessionKeepAliveInterval = 4 * time.Minute

// SessionTransport authenticates via FlexiBee v2 session auth, bypassing the
// rest-api-rw licence check that blocks HTTP Basic Auth on basic2016 licences.
type SessionTransport struct {
	base     *transportBase
	username string
	password string // never log

	mu            sync.Mutex
	authSessionID string
	csrfToken     string
	keepAliveStop chan struct{} // closed to stop the keep-alive goroutine
}

// session holds the active session credentials.
type session struct {
	AuthSessionID string `json:"authSessionId"`
	CSRFToken     string `json:"csrfToken"`
	Success       bool   `json:"success"`
}

// NewSessionTransport constructs a SessionTransport from the validated config.
func NewSessionTransport(cfg *config.Config) (*SessionTransport, error) {
	base, err := newTransportBase(cfg, sessionRedirect(cfg.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("session transport: %w", err)
	}
	return &SessionTransport{
		base:     base,
		username: cfg.Username,
		password: cfg.Password,
	}, nil
}

// Name implements Transport.
func (t *SessionTransport) Name() string { return "session" }

// Get implements Transport.
func (t *SessionTransport) Get(ctx context.Context, path string) ([]byte, error) {
	return t.doSession(ctx, http.MethodGet, path, nil)
}

// Post implements Transport.
func (t *SessionTransport) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return t.doSession(ctx, http.MethodPost, path, body)
}

// Put implements Transport.
func (t *SessionTransport) Put(ctx context.Context, path string, body []byte) ([]byte, error) {
	return t.doSession(ctx, http.MethodPut, path, body)
}

// Delete implements Transport.
func (t *SessionTransport) Delete(ctx context.Context, path string) ([]byte, error) {
	return t.doSession(ctx, http.MethodDelete, path, nil)
}

// ── Session lifecycle ─────────────────────────────────────────────────────────

// login performs the one-time authentication.  It must only be called when no
// session is active (i.e. when authSessionID is empty).
//
// Protocol:
//
//	POST /v2/login-logout/login
//	Content-Type: application/x-www-form-urlencoded
//	Body: username=...&password=...&licenseChecked=false
//	→ 200 {"authSessionId":"...", "csrfToken":"...", "success":true}
func (t *SessionTransport) login(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", t.username)
	form.Set("password", t.password)
	form.Set("licenseChecked", "false")

	loginURL := t.base.baseURL + "/v2/login-logout/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return NewTransportError("building login request: "+err.Error(), err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", sessionUserAgent)

	resp, err := t.base.httpClient.Do(req)
	if err != nil {
		return NewTransportError("login request failed: "+err.Error(), err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return NewHTTPError(resp.StatusCode, body)
	}

	var sess session
	if err := json.Unmarshal(body, &sess); err != nil {
		return NewTransportError("parsing login response: "+err.Error(), err)
	}
	if !sess.Success {
		return NewAuthError("login failed: server returned success=false")
	}
	if sess.AuthSessionID == "" {
		return NewAuthError("login failed: server returned empty authSessionId")
	}

	t.authSessionID = sess.AuthSessionID
	t.csrfToken = sess.CSRFToken

	// Start the keep-alive goroutine for this session.
	t.startKeepAlive()

	return nil
}

// startKeepAlive launches a background goroutine that pings the keep-alive
// endpoint every sessionKeepAliveInterval to prevent session expiry.
// Any existing keep-alive goroutine is stopped first.
func (t *SessionTransport) startKeepAlive() {
	// Stop any existing keep-alive goroutine.
	if t.keepAliveStop != nil {
		close(t.keepAliveStop)
	}
	stop := make(chan struct{})
	t.keepAliveStop = stop

	go func() {
		ticker := time.NewTicker(sessionKeepAliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				t.pingKeepAlive()
			}
		}
	}()
}

// pingKeepAlive sends a keep-alive ping to extend the session lifetime.
// Errors are silently swallowed — if the session has expired the next real
// request will detect a 401 and trigger a re-login.
func (t *SessionTransport) pingKeepAlive() {
	t.mu.Lock()
	sid := t.authSessionID
	csrf := t.csrfToken
	t.mu.Unlock()

	if sid == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		t.base.baseURL+"/v2/login-logout/session-keep-alive.json", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", sessionUserAgent)
	req.Header.Set("Cookie", buildCookieHeader(sid, csrf))

	resp, err := t.base.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.ReadAll(resp.Body)
}

// ensureSession calls login if no active session exists.  Thread-safe.
func (t *SessionTransport) ensureSession(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.authSessionID != "" {
		return nil
	}
	return t.login(ctx)
}

// invalidateSession clears the session so the next request triggers a re-login.
// Thread-safe.
func (t *SessionTransport) invalidateSession() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.authSessionID = ""
	t.csrfToken = ""
	if t.keepAliveStop != nil {
		close(t.keepAliveStop)
		t.keepAliveStop = nil
	}
}

// ── Core request dispatch ─────────────────────────────────────────────────────

// doSession executes an HTTP request using the active session.
// On session expiry (401 + remoteLogoutDemand) it re-logs in and retries once.
func (t *SessionTransport) doSession(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if err := t.ensureSession(ctx); err != nil {
		return nil, err
	}

	b, err := t.execSession(ctx, method, path, body)
	if err != nil {
		// Session expired or was invalidated remotely → single retry.
		if isSessionExpired(err) {
			t.invalidateSession()
			if loginErr := t.ensureSession(ctx); loginErr != nil {
				return nil, loginErr
			}
			return t.execSession(ctx, method, path, body)
		}
		return nil, err
	}
	return b, nil
}

// execSession executes one HTTP request with the current session credentials.
// Writes include the CSRF token as a query parameter (?token=<csrf>).
func (t *SessionTransport) execSession(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	t.mu.Lock()
	sid := t.authSessionID
	csrf := t.csrfToken
	t.mu.Unlock()

	// All session-mode paths use /v2/ prefix.
	v2path := ensureV2Prefix(path)

	// Write methods require the CSRF token as a query parameter.
	if isWriteMethod(method) && csrf != "" {
		if strings.Contains(v2path, "?") {
			v2path += "&token=" + url.QueryEscape(csrf)
		} else {
			v2path += "?token=" + url.QueryEscape(csrf)
		}
	}

	rawURL := t.base.baseURL + v2path
	req, err := newJSONRequest(ctx, method, rawURL, body)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("building %s request for %q: %v", method, path, err))
	}

	// Session auth: cookies
	req.Header.Set("Cookie", buildCookieHeader(sid, csrf))
	req.Header.Set("User-Agent", sessionUserAgent)

	return doHTTP(ctx, t.base.httpClient, req)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// buildCookieHeader constructs the Cookie header for session requests.
func buildCookieHeader(authSessionID, csrfToken string) string {
	parts := []string{"authSessionId=" + authSessionID}
	if csrfToken != "" {
		parts = append(parts, "CSRF-Token="+csrfToken)
	}
	return strings.Join(parts, "; ")
}

// ensureV2Prefix prepends "/v2" to path if it does not already start with
// "/v2" or "/v3".  The /v2 prefix is required to create a ClientType.FLEXI
// session that bypasses the API licence check.
func ensureV2Prefix(path string) string {
	if strings.HasPrefix(path, "/v2") || strings.HasPrefix(path, "/v3") {
		return path
	}
	return "/v2" + path
}

// isWriteMethod reports whether method mutates server state.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	}
	return false
}

// isSessionExpired reports whether err indicates an expired or invalidated
// session.  FlexiBee returns 401 with a body containing "remoteLogoutDemand"
// when another session for the same user+clientType has been created.
func isSessionExpired(err error) bool {
	if err == nil {
		return false
	}
	fe, ok := toFlexiError(err)
	if !ok {
		return false
	}
	return fe.Kind == ErrAuth
}

// toFlexiError extracts a *FlexiError from err, following the Unwrap chain.
func toFlexiError(err error) (*FlexiError, bool) {
	var fe *FlexiError
	if fe, ok := err.(*FlexiError); ok { //nolint:errorlint
		return fe, true
	}
	_ = fe
	return nil, false
}

// bodyReader returns an io.Reader for body, or nil when body is empty.
func bodyReader(b []byte) io.Reader {
	if len(b) == 0 {
		return nil
	}
	return bytes.NewReader(b)
}
