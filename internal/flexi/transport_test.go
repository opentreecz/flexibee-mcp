// Package flexi – transport_test.go exercises APITransport, SessionTransport,
// and AutoTransport against an httptest server.
package flexi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/config"
)

// ─── Test server ──────────────────────────────────────────────────────────────

// flexiServer is a minimal httptest.Server that mimics FlexiBee REST API
// behaviour for transport-layer tests.
type flexiServer struct {
	srv *httptest.Server
	// counters for asserting call counts
	loginCalls     atomic.Int32
	keepAliveCalls atomic.Int32
	apiCalls       atomic.Int32

	// authMode controls whether Basic Auth is accepted or returns 402.
	// "block" (default) returns 402 for all Basic Auth requests.
	// "allow" accepts Basic Auth.
	authMode string

	// sessionID is the session cookie value returned after login.
	sessionID string
	csrfToken string
}

// companiesJSON is the minimal /c.json response.
const companiesJSON = `{"companies":{"company":[{"dbNazev":"test-co","nazev":"Test Company"}]}}`

// newFlexiServer builds a mock FlexiBee server.
func newFlexiServer(t *testing.T, authMode string) *flexiServer {
	t.Helper()
	fs := &flexiServer{
		authMode:  authMode,
		sessionID: "test-session-id",
		csrfToken: "test-csrf-token",
	}

	mux := http.NewServeMux()

	// ── Login endpoint ──────────────────────────────────────────────────────
	mux.HandleFunc("/v2/login-logout/login", func(w http.ResponseWriter, r *http.Request) {
		fs.loginCalls.Add(1)
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"authSessionId": fs.sessionID,
			"csrfToken":     fs.csrfToken,
			"success":       true,
		})
	})

	// ── Keep-alive endpoint ─────────────────────────────────────────────────
	mux.HandleFunc("/v2/login-logout/session-keep-alive.json", func(w http.ResponseWriter, r *http.Request) {
		fs.keepAliveCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"success":true}`)
	})

	// ── All other /v2/ paths: session-authenticated ─────────────────────────
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("authSessionId")
		if cookie == nil || cookie.Value != fs.sessionID {
			http.Error(w, "no session", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, companiesJSON)
	})

	// ── /c.json and other root paths: Basic Auth ────────────────────────────
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fs.apiCalls.Add(1)
		_, _, hasAuth := r.BasicAuth()
		if fs.authMode == "block" || !hasAuth {
			http.Error(w, `{"winstrom":{"@globalError":"PaymentRequired"}}`, http.StatusPaymentRequired)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, companiesJSON)
	})

	fs.srv = httptest.NewTLSServer(mux)
	t.Cleanup(fs.srv.Close)
	return fs
}

// cfg returns a Config pointing at the test server.
func (fs *flexiServer) cfg(authMode string) *config.Config {
	return &config.Config{
		BaseURL:       fs.srv.URL,
		Username:      "testuser",
		Password:      "testpass",
		TimeoutMS:     5000,
		PageSize:      50,
		AuthMode:      authMode,
		SkipTLSVerify: true, // httptest.NewTLSServer uses a self-signed cert
	}
}

// ─── APITransport tests ───────────────────────────────────────────────────────

func TestAPITransportGetSuccess(t *testing.T) {
	fs := newFlexiServer(t, "allow")
	tr, err := NewAPITransport(fs.cfg(config.AuthModeAPI))
	if err != nil {
		t.Fatalf("NewAPITransport: %v", err)
	}

	body, err := tr.Get(context.Background(), "/c.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(body), "test-co") {
		t.Errorf("body = %q; want to contain \"test-co\"", body)
	}
}

func TestAPITransportReturnsPaymentRequired(t *testing.T) {
	fs := newFlexiServer(t, "block") // always returns 402
	tr, err := NewAPITransport(fs.cfg(config.AuthModeAPI))
	if err != nil {
		t.Fatalf("NewAPITransport: %v", err)
	}

	_, err = tr.Get(context.Background(), "/c.json")
	if err == nil {
		t.Fatal("expected ErrPaymentRequired, got nil")
	}
	fe, ok := err.(*FlexiError)
	if !ok {
		t.Fatalf("err type = %T, want *FlexiError", err)
	}
	if fe.Kind != ErrPaymentRequired {
		t.Errorf("Kind = %v, want ErrPaymentRequired", fe.Kind)
	}
}

func TestAPITransportContextCancellation(t *testing.T) {
	fs := newFlexiServer(t, "allow")
	tr, err := NewAPITransport(fs.cfg(config.AuthModeAPI))
	if err != nil {
		t.Fatalf("NewAPITransport: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = tr.Get(ctx, "/c.json")
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

func TestAPITransportSameOriginRedirectAllowed(t *testing.T) {
	// Build a server that redirects /old → /new (same origin).
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"companies":{"company":[]}}`)
	})
	srv = httptest.NewTLSServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		BaseURL: srv.URL, Username: "u", Password: "p",
		TimeoutMS: 5000, PageSize: 50, AuthMode: config.AuthModeAPI,
		SkipTLSVerify: true,
	}
	tr, err := NewAPITransport(cfg)
	if err != nil {
		t.Fatalf("NewAPITransport: %v", err)
	}
	_, err = tr.Get(context.Background(), "/old")
	if err != nil {
		t.Errorf("same-origin redirect should succeed, got: %v", err)
	}
}

func TestAPITransportCrossOriginRedirectRefused(t *testing.T) {
	// Build a server that redirects to a different host.
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/", http.StatusMovedPermanently)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		BaseURL: srv.URL, Username: "u", Password: "p",
		TimeoutMS: 5000, PageSize: 50, AuthMode: config.AuthModeAPI,
		SkipTLSVerify: true,
	}
	tr, err := NewAPITransport(cfg)
	if err != nil {
		t.Fatalf("NewAPITransport: %v", err)
	}
	_, err = tr.Get(context.Background(), "/redirect")
	if err == nil {
		t.Fatal("cross-origin redirect should be refused, got nil error")
	}
	if !strings.Contains(err.Error(), "cross-origin") {
		t.Errorf("error = %q; want to contain \"cross-origin\"", err)
	}
}

func TestAPITransportName(t *testing.T) {
	fs := newFlexiServer(t, "allow")
	tr, err := NewAPITransport(fs.cfg(config.AuthModeAPI))
	if err != nil {
		t.Fatalf("NewAPITransport: %v", err)
	}
	if tr.Name() != "api" {
		t.Errorf("Name = %q, want \"api\"", tr.Name())
	}
}

// ─── SessionTransport tests ───────────────────────────────────────────────────

func TestSessionTransportLoginOnFirstRequest(t *testing.T) {
	fs := newFlexiServer(t, "block")
	tr, err := NewSessionTransport(fs.cfg(config.AuthModeSession))
	if err != nil {
		t.Fatalf("NewSessionTransport: %v", err)
	}

	body, err := tr.Get(context.Background(), "/c.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(body), "test-co") {
		t.Errorf("body = %q; want to contain \"test-co\"", body)
	}
	if fs.loginCalls.Load() != 1 {
		t.Errorf("loginCalls = %d, want 1", fs.loginCalls.Load())
	}
}

func TestSessionTransportSingleLogin(t *testing.T) {
	// Multiple requests must share a single login.
	fs := newFlexiServer(t, "block")
	tr, err := NewSessionTransport(fs.cfg(config.AuthModeSession))
	if err != nil {
		t.Fatalf("NewSessionTransport: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := tr.Get(context.Background(), "/c.json"); err != nil {
			t.Fatalf("Get %d: %v", i, err)
		}
	}
	if n := fs.loginCalls.Load(); n != 1 {
		t.Errorf("loginCalls = %d, want 1 (single login for all requests)", n)
	}
}

func TestSessionTransportCSRFTokenOnWrite(t *testing.T) {
	// Verify that PUT/DELETE requests include ?token=<csrf> in the URL.
	var capturedURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/login-logout/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"authSessionId": "sid123",
			"csrfToken":     "csrf456",
			"success":       true,
		})
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"winstrom":{}}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		BaseURL: srv.URL, Username: "u", Password: "p",
		TimeoutMS: 5000, PageSize: 50, AuthMode: config.AuthModeSession,
		SkipTLSVerify: true,
	}
	tr, err := NewSessionTransport(cfg)
	if err != nil {
		t.Fatalf("NewSessionTransport: %v", err)
	}

	_, err = tr.Put(context.Background(), "/c/demo/adresar.json", []byte(`{}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !strings.Contains(capturedURL, "token=csrf456") {
		t.Errorf("capturedURL = %q; want to contain \"token=csrf456\"", capturedURL)
	}
}

func TestSessionTransportV2PrefixAdded(t *testing.T) {
	// All paths must be prefixed with /v2/ by the session transport.
	var capturedPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/login-logout/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"authSessionId": "sid", "csrfToken": "tok", "success": true,
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, companiesJSON)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		BaseURL: srv.URL, Username: "u", Password: "p",
		TimeoutMS: 5000, PageSize: 50, AuthMode: config.AuthModeSession,
		SkipTLSVerify: true,
	}
	tr, err := NewSessionTransport(cfg)
	if err != nil {
		t.Fatalf("NewSessionTransport: %v", err)
	}

	_, _ = tr.Get(context.Background(), "/c.json")

	if !strings.HasPrefix(capturedPath, "/v2/") {
		t.Errorf("capturedPath = %q; want /v2/ prefix", capturedPath)
	}
}

func TestSessionTransportReloginOn401(t *testing.T) {
	// After a 401 response the transport should re-login and retry once.
	loginCount := atomic.Int32{}
	requestCount := atomic.Int32{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/login-logout/login", func(w http.ResponseWriter, r *http.Request) {
		n := loginCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// Return a fresh session ID each time.
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"authSessionId": fmt.Sprintf("sid%d", n),
			"csrfToken":     "tok",
			"success":       true,
		})
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		// First request: return 401 (session expired).
		// Second request: return 200.
		if n == 1 {
			http.Error(w, "session expired", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, companiesJSON)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	cfg := &config.Config{
		BaseURL: srv.URL, Username: "u", Password: "p",
		TimeoutMS: 5000, PageSize: 50, AuthMode: config.AuthModeSession,
		SkipTLSVerify: true,
	}
	tr, err := NewSessionTransport(cfg)
	if err != nil {
		t.Fatalf("NewSessionTransport: %v", err)
	}

	body, err := tr.Get(context.Background(), "/c.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(body), "test-co") {
		t.Errorf("body = %q; want to contain \"test-co\"", body)
	}
	if n := loginCount.Load(); n != 2 {
		t.Errorf("loginCalls = %d, want 2 (initial login + re-login)", n)
	}
}

func TestSessionTransportName(t *testing.T) {
	fs := newFlexiServer(t, "block")
	tr, err := NewSessionTransport(fs.cfg(config.AuthModeSession))
	if err != nil {
		t.Fatalf("NewSessionTransport: %v", err)
	}
	if tr.Name() != "session" {
		t.Errorf("Name = %q, want \"session\"", tr.Name())
	}
}

// ─── AutoTransport tests ──────────────────────────────────────────────────────

func TestAutoTransportFallsBackToSession(t *testing.T) {
	// API returns 402 → AutoTransport should fall back to session transport.
	fs := newFlexiServer(t, "block") // block=API returns 402
	cfg := fs.cfg(config.AuthModeAuto)

	tr, err := NewAutoTransport(cfg)
	if err != nil {
		t.Fatalf("NewAutoTransport: %v", err)
	}

	body, err := tr.Get(context.Background(), "/c.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(body), "test-co") {
		t.Errorf("body = %q; want to contain \"test-co\"", body)
	}
	// Login should have been called (session transport activated).
	if fs.loginCalls.Load() == 0 {
		t.Error("expected session login to be called during auto fallback, got 0")
	}
}

func TestAutoTransportUsesAPIWhenAvailable(t *testing.T) {
	// API returns 200 → AutoTransport should not fall back.
	fs := newFlexiServer(t, "allow") // allow=API accepts Basic Auth
	cfg := fs.cfg(config.AuthModeAuto)

	tr, err := NewAutoTransport(cfg)
	if err != nil {
		t.Fatalf("NewAutoTransport: %v", err)
	}

	body, err := tr.Get(context.Background(), "/c.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(body), "test-co") {
		t.Errorf("body = %q; want to contain \"test-co\"", body)
	}
	if fs.loginCalls.Load() != 0 {
		t.Errorf("loginCalls = %d, want 0 (API transport should have been used)", fs.loginCalls.Load())
	}
}

func TestAutoTransportNameReflectsActive(t *testing.T) {
	fs := newFlexiServer(t, "block")
	cfg := fs.cfg(config.AuthModeAuto)

	tr, err := NewAutoTransport(cfg)
	if err != nil {
		t.Fatalf("NewAutoTransport: %v", err)
	}

	// Before any call the name says "api" (first transport in the list).
	if !strings.Contains(tr.Name(), "api") {
		t.Errorf("Name before first call = %q; want to contain \"api\"", tr.Name())
	}

	// After a call that causes fallback it should reflect "session".
	if _, err := tr.Get(context.Background(), "/c.json"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(tr.Name(), "session") {
		t.Errorf("Name after fallback = %q; want to contain \"session\"", tr.Name())
	}
}

func TestAutoTransportExplicitAPIMode(t *testing.T) {
	// AuthModeAPI returns the APITransport directly, not an AutoTransport.
	fs := newFlexiServer(t, "allow")
	cfg := fs.cfg(config.AuthModeAPI)

	tr, err := NewAutoTransport(cfg)
	if err != nil {
		t.Fatalf("NewAutoTransport: %v", err)
	}
	if tr.Name() != "api" {
		t.Errorf("Name = %q, want \"api\"", tr.Name())
	}
}

func TestAutoTransportExplicitSessionMode(t *testing.T) {
	fs := newFlexiServer(t, "block")
	cfg := fs.cfg(config.AuthModeSession)

	tr, err := NewAutoTransport(cfg)
	if err != nil {
		t.Fatalf("NewAutoTransport: %v", err)
	}
	if tr.Name() != "session" {
		t.Errorf("Name = %q, want \"session\"", tr.Name())
	}
}

// ─── Client integration ───────────────────────────────────────────────────────

func TestClientListCompaniesViaSession(t *testing.T) {
	// Full Client using session transport against the mock server.
	fs := newFlexiServer(t, "block")
	cfg := fs.cfg(config.AuthModeSession)

	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	companies, err := c.ListCompanies(context.Background())
	if err != nil {
		t.Fatalf("ListCompanies: %v", err)
	}
	if len(companies) != 1 || companies[0].DBNazev != "test-co" {
		t.Errorf("companies = %+v, want [{DBNazev:test-co}]", companies)
	}
}

func TestClientListCompaniesAutoFallback(t *testing.T) {
	// Auto mode: API 402 → session fallback → returns data.
	fs := newFlexiServer(t, "block")
	cfg := fs.cfg(config.AuthModeAuto)

	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	companies, err := c.ListCompanies(context.Background())
	if err != nil {
		t.Fatalf("ListCompanies: %v", err)
	}
	if len(companies) == 0 {
		t.Error("expected at least one company via auto fallback, got 0")
	}
	if c.TransportName() == "" {
		t.Error("TransportName should not be empty after a successful call")
	}
}

// ─── Helper: timeout handling ─────────────────────────────────────────────────

func TestAPITransportTimeout(t *testing.T) {
	// Server that hangs longer than the client timeout.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.Config{
		BaseURL:  srv.URL,
		Username: "u", Password: "p",
		TimeoutMS:     50, // 50 ms – shorter than the server sleep
		PageSize:      50,
		AuthMode:      config.AuthModeAPI,
		SkipTLSVerify: true,
	}
	tr, err := NewAPITransport(cfg)
	if err != nil {
		t.Fatalf("NewAPITransport: %v", err)
	}

	_, err = tr.Get(context.Background(), "/c.json")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
