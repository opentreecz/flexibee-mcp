// Package flexi – transport_api.go implements the HTTP Basic Auth REST transport.
package flexi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/opentreecz/flexibee-mcp/internal/config"
)

// apiUserAgent is the User-Agent header sent by the API transport.
const apiUserAgent = "flexibee-mcp/0.1 (https://github.com/opentreecz/flexibee-mcp)"

// APITransport is a Transport that uses HTTP Basic Authentication.
//
// It is the primary transport when the FlexiBee licence includes rest-api-rw.
// It returns ErrPaymentRequired (HTTP 402) when that licence key is absent,
// which causes AutoTransport to fall back to the session transport.
type APITransport struct {
	base     *transportBase
	username string
	password string // never log
}

// NewAPITransport constructs an APITransport from the validated config.
func NewAPITransport(cfg *config.Config) (*APITransport, error) {
	base, err := newTransportBase(cfg, sameOriginRedirect(cfg.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("api transport: %w", err)
	}
	return &APITransport{
		base:     base,
		username: cfg.Username,
		password: cfg.Password,
	}, nil
}

// Name implements Transport.
func (t *APITransport) Name() string { return "api" }

// Get implements Transport.
func (t *APITransport) Get(ctx context.Context, path string) ([]byte, error) {
	return t.do(ctx, http.MethodGet, path, nil)
}

// Post implements Transport.
func (t *APITransport) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return t.do(ctx, http.MethodPost, path, body)
}

// Put implements Transport.
func (t *APITransport) Put(ctx context.Context, path string, body []byte) ([]byte, error) {
	return t.do(ctx, http.MethodPut, path, body)
}

// Delete implements Transport.
func (t *APITransport) Delete(ctx context.Context, path string) ([]byte, error) {
	return t.do(ctx, http.MethodDelete, path, nil)
}

// do executes an HTTP request with Basic authentication.
func (t *APITransport) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	rawURL := t.base.baseURL + path
	req, err := newJSONRequest(ctx, method, rawURL, body)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("building %s request for %q: %v", method, path, err))
	}
	req.SetBasicAuth(t.username, t.password)
	req.Header.Set("User-Agent", apiUserAgent)

	return doHTTP(ctx, t.base.httpClient, req)
}

// newJSONRequest builds an *http.Request with Accept: application/json and,
// when body is non-nil, Content-Type: application/json.
func newJSONRequest(ctx context.Context, method, rawURL string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
