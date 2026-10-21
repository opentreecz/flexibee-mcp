// Package flexi – transport.go defines the Transport interface and the
// auto-fallback transport that tries each implementation in priority order.
package flexi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/config"
)

// Transport abstracts the HTTP communication mechanism used to reach FlexiBee.
//
// All paths passed to Transport methods are relative to the configured base
// URL.  The transport is responsible for authentication, retries within a
// single request (e.g. session refresh), and returning the raw response body
// on success.
//
// A Transport must be safe for concurrent use by multiple goroutines.
type Transport interface {
	// Get performs an HTTP GET to the given path.
	Get(ctx context.Context, path string) ([]byte, error)
	// Post performs an HTTP POST with a JSON body.
	Post(ctx context.Context, path string, body []byte) ([]byte, error)
	// Put performs an HTTP PUT with a JSON body.
	Put(ctx context.Context, path string, body []byte) ([]byte, error)
	// Delete performs an HTTP DELETE to the given path.
	Delete(ctx context.Context, path string) ([]byte, error)
	// Name returns a short human-readable identifier (for logging/errors).
	Name() string
}

// ── transportBase – shared HTTP infrastructure ─────────────────────────────

// transportBase holds the shared *http.Client and base URL used by the API
// and Session transports.  The Java transport uses a subprocess instead.
type transportBase struct {
	httpClient *http.Client
	baseURL    string
	cfg        *config.Config
}

// newTransportBase builds a reusable *http.Client with the configured TLS
// settings, timeout, and redirect policy.
func newTransportBase(cfg *config.Config, checkRedirect func(*http.Request, []*http.Request) error) (*transportBase, error) {
	t, err := buildTransportHTTP(cfg)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{
		Timeout:       time.Duration(cfg.TimeoutMS) * time.Millisecond,
		Transport:     t,
		CheckRedirect: checkRedirect,
	}
	return &transportBase{
		httpClient: hc,
		baseURL:    cfg.BaseURL,
		cfg:        cfg,
	}, nil
}

// buildTransportHTTP returns a cloned http.DefaultTransport with optional
// private CA and optional TLS skip-verify (dev only).
func buildTransportHTTP(cfg *config.Config) (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("http.DefaultTransport is not *http.Transport")
	}
	tr := base.Clone()

	if cfg.SkipTLSVerify {
		tlsCfg := tr.TLSClientConfig
		if tlsCfg == nil {
			tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
		} else {
			tlsCfg = tlsCfg.Clone()
		}
		//nolint:gosec // intentional for dev/self-signed
		tlsCfg.InsecureSkipVerify = true
		tr.TLSClientConfig = tlsCfg
		return tr, nil
	}

	if cfg.CAFile == "" {
		return tr, nil
	}

	pemData, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file %q: %w", cfg.CAFile, err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("CA file %q: no valid PEM certificates found", cfg.CAFile)
	}

	tlsCfg := tr.TLSClientConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		tlsCfg = tlsCfg.Clone()
	}
	tlsCfg.RootCAs = pool
	tr.TLSClientConfig = tlsCfg
	return tr, nil
}

// ── redirect guards ───────────────────────────────────────────────────────────

// sameOriginRedirect builds a checkRedirect function that enforces:
//   - max maxRedirects hops,
//   - same-origin only,
//   - no HTTPS→HTTP downgrade.
func sameOriginRedirect(baseURL string) func(*http.Request, []*http.Request) error {
	const max = 5
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= max {
			return fmt.Errorf("flexibee: too many redirects (max %d)", max)
		}
		origin, err := url.Parse(baseURL)
		if err != nil {
			return fmt.Errorf("flexibee: invalid base URL: %w", err)
		}
		if origin.Scheme == "https" && req.URL.Scheme == "http" {
			return fmt.Errorf("flexibee: redirect refused: HTTPS→HTTP downgrade to %s",
				req.URL.Redacted())
		}
		if !sameOriginURLs(origin, req.URL) {
			return fmt.Errorf("flexibee: redirect refused: cross-origin to %s",
				req.URL.Redacted())
		}
		return nil
	}
}

// sessionRedirect is like sameOriginRedirect but also stops when the redirect
// target is the login page (to avoid login-redirect loops in session mode).
func sessionRedirect(baseURL string) func(*http.Request, []*http.Request) error {
	base := sameOriginRedirect(baseURL)
	return func(req *http.Request, via []*http.Request) error {
		if strings.Contains(req.URL.Path, "login-logout") {
			return http.ErrUseLastResponse
		}
		return base(req, via)
	}
}

// sameOriginURLs reports whether a and b share scheme, host, and effective port.
func sameOriginURLs(a, b *url.URL) bool {
	return a.Scheme == b.Scheme &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePortURL(a) == effectivePortURL(b)
}

// effectivePortURL returns the explicit port, or the default for the scheme.
func effectivePortURL(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch u.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// ── doHTTP – low-level HTTP helper ────────────────────────────────────────────

// doHTTP executes a single HTTP request using the provided client, returning
// the raw response body on 2xx.  It never logs credentials or response bodies.
func doHTTP(ctx context.Context, hc *http.Client, req *http.Request) ([]byte, error) {
	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, NewTransportError(fmt.Sprintf("%s %q: %v", req.Method, req.URL.Path, err), err)
		}
		return nil, NewTransportError(fmt.Sprintf("%s %q: %v", req.Method, req.URL.Path, err), err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return body, nil
	}
	return nil, NewHTTPError(resp.StatusCode, body)
}

// readBody reads resp.Body safely.
func readBody(resp *http.Response) ([]byte, error) {
	b := &bytes.Buffer{}
	if _, err := b.ReadFrom(resp.Body); err != nil {
		return nil, NewTransportError(
			fmt.Sprintf("reading response body from %s: %v", resp.Request.URL.Path, err), err)
	}
	return b.Bytes(), nil
}

// ── AutoTransport – priority-ordered fallback ─────────────────────────────────

// AutoTransport tries a list of Transport implementations in order.
// On a 402 (payment required / no licence) from the first transport, it
// advances to the next without surfacing the error.  Other errors propagate
// immediately from whichever transport is currently active.
type AutoTransport struct {
	transports []Transport
	active     int // index of the currently active transport
}

// NewAutoTransport builds an AutoTransport from the config.
// Priority order: API → Session → Java (if available).
func NewAutoTransport(cfg *config.Config) (Transport, error) {
	switch cfg.AuthMode {
	case config.AuthModeAPI:
		return NewAPITransport(cfg)
	case config.AuthModeSession:
		return NewSessionTransport(cfg)
	case config.AuthModeJava:
		return NewJavaTransport(cfg)
	}

	// AuthModeAuto: build all available transports in priority order.
	var ts []Transport

	api, err := NewAPITransport(cfg)
	if err != nil {
		return nil, fmt.Errorf("auto transport: building API transport: %w", err)
	}
	ts = append(ts, api)

	sess, err := NewSessionTransport(cfg)
	if err != nil {
		return nil, fmt.Errorf("auto transport: building session transport: %w", err)
	}
	ts = append(ts, sess)

	// Java transport is optional – skip if unavailable.
	javaT, javaErr := NewJavaTransport(cfg)
	if javaErr == nil {
		ts = append(ts, javaT)
	}

	return &AutoTransport{transports: ts}, nil
}

// Name returns the active transport's name.
func (a *AutoTransport) Name() string {
	if a.active < len(a.transports) {
		return "auto[" + a.transports[a.active].Name() + "]"
	}
	return "auto[exhausted]"
}

// Get implements Transport.
func (a *AutoTransport) Get(ctx context.Context, path string) ([]byte, error) {
	return a.do(ctx, func(t Transport) ([]byte, error) { return t.Get(ctx, path) })
}

// Post implements Transport.
func (a *AutoTransport) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return a.do(ctx, func(t Transport) ([]byte, error) { return t.Post(ctx, path, body) })
}

// Put implements Transport.
func (a *AutoTransport) Put(ctx context.Context, path string, body []byte) ([]byte, error) {
	return a.do(ctx, func(t Transport) ([]byte, error) { return t.Put(ctx, path, body) })
}

// Delete implements Transport.
func (a *AutoTransport) Delete(ctx context.Context, path string) ([]byte, error) {
	return a.do(ctx, func(t Transport) ([]byte, error) { return t.Delete(ctx, path) })
}

// do tries transports in order, advancing on 402.
func (a *AutoTransport) do(ctx context.Context, call func(Transport) ([]byte, error)) ([]byte, error) {
	for i := a.active; i < len(a.transports); i++ {
		b, err := call(a.transports[i])
		if err == nil {
			a.active = i // remember which transport succeeded
			return b, nil
		}
		// 402 Payment Required – this transport is blocked by licence.
		// Try the next transport.
		if isPaymentRequired(err) {
			a.active = i + 1
			continue
		}
		// Any other error – surface it from the current transport.
		return nil, err
	}
	return nil, &FlexiError{
		Kind:    ErrTransport,
		Message: "all transports exhausted – could not connect to FlexiBee",
	}
}

// isPaymentRequired reports whether err is a 402 HTTP error.
func isPaymentRequired(err error) bool {
	var fe *FlexiError
	if errors.As(err, &fe) {
		return fe.Kind == ErrPaymentRequired
	}
	return false
}
