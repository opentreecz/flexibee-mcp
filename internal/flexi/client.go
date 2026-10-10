// Package flexi provides the FlexiBee HTTP client, typed filter tree, and
// query builder used by the MCP tool layer.
//
// Package layout:
//
//   - client.go  – FilterNode, RawFilter, SortField, QueryOptions, Client (this file)
//   - query.go   – filter implementations, Query, QueryResult, URL/body builders
//   - envelopes.go – response envelope types (Company, Evidence, Property, …)
//   - errors.go  – FlexiError, ErrorKind, constructors, ParseAppErrors
package flexi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/config"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	// userAgent is sent as the User-Agent header on every outbound request.
	userAgent = "flexibee-mcp/0.1 (https://github.com/opentreecz/flexibee-mcp)"

	// maxRedirects is the maximum number of same-origin redirects followed per
	// request.  FlexiBee uses HTTP 301 to redirect non-internal identifiers to
	// their canonical internal-ID URL.
	maxRedirects = 5
)

// ── Filter tree interface ─────────────────────────────────────────────────────

// FilterNode is the sealed interface for all Flexi filter tree nodes.
//
// The unexported flexi() method prevents external packages from satisfying the
// interface, keeping the set of valid node types closed within this package.
// Build() serialises any node to a Flexi filter expression string suitable for
// embedding in a URL path component or a POST body.
//
// Concrete typed node implementations (CompareFilter, AndFilter, …) are
// defined in query.go.  Use FilterExpr to wrap a pre-rendered string.
type FilterNode interface {
	flexi()        // unexported marker – restricts implementations to this package
	Build() string // serialises the node to a Flexi filter expression string
}

// RawFilter wraps a pre-built Flexi filter expression string as a FilterNode.
// It is the escape hatch for callers that construct expressions outside the
// typed filter builder.  Use FilterExpr to construct one.
type RawFilter string

func (f RawFilter) flexi()        {}
func (f RawFilter) Build() string { return string(f) }

// FilterExpr wraps a raw Flexi filter expression string as a FilterNode.
// Returns nil when expr is empty; nil is treated as "no filter" throughout.
func FilterExpr(expr string) FilterNode {
	if expr == "" {
		return nil
	}
	return RawFilter(expr)
}

// ── Sort ──────────────────────────────────────────────────────────────────────

// SortField pairs a field name with a sort direction for ORDER BY clauses.
type SortField struct {
	Field string
	Desc  bool // true = descending (@D), false = ascending (@A)
}

// ── Query options ─────────────────────────────────────────────────────────────

// QueryOptions holds the complete set of parameters for a bounded record query
// passed to Client.QueryRecords.
type QueryOptions struct {
	Company  string
	Evidence string
	// Filter is the optional filter tree; nil means no filter.
	Filter FilterNode
	// Fields restricts the projected fields; nil or empty means all fields.
	Fields []string
	// Sort specifies the ordering; nil means server default.
	Sort []SortField
	// Limit is the maximum number of records per page; 0 defers to PageSize.
	Limit int
	// Start is the zero-based offset for pagination.
	Start int
	// AddCount requests the total filtered row count (@rowCount) in the
	// response envelope.
	AddCount bool
}

// ── HTTP client ───────────────────────────────────────────────────────────────

// Client is an authenticated FlexiBee REST API HTTP client.
//
// A single Client encapsulates one account's credentials.  Credentials are
// applied on every request via HTTP Basic authentication and are never
// written to any log output.
type Client struct {
	httpClient *http.Client
	baseURL    string
	username   string
	password   string // never log
	userAgent  string
	cfg        *config.Config
}

// NewClient constructs a Client from the validated configuration.
//
// It creates exactly one *http.Client with:
//   - a bounded timeout derived from cfg.TimeoutMS,
//   - a cloned default transport (connection reuse, HTTP/2 where available),
//   - an optional private CA bundle appended to the system cert pool, and
//   - a same-origin redirect policy capped at maxRedirects hops.
func NewClient(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, NewConfigError("config must not be nil")
	}
	if cfg.BaseURL == "" {
		return nil, NewConfigError("base URL is required")
	}

	transport, err := buildTransport(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("flexi.NewClient: %w", err)
	}

	c := &Client{
		baseURL:   cfg.BaseURL,
		username:  cfg.Username,
		password:  cfg.Password,
		userAgent: userAgent,
		cfg:       cfg,
	}
	c.httpClient = &http.Client{
		Timeout:       time.Duration(cfg.TimeoutMS) * time.Millisecond,
		Transport:     transport,
		CheckRedirect: c.checkRedirect,
	}
	return c, nil
}

// buildTransport returns a cloned http.DefaultTransport, optionally extended
// with a private CA certificate bundle appended to the system cert pool.
func buildTransport(caFile string) (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("http.DefaultTransport is not *http.Transport")
	}
	transport := base.Clone()

	if caFile == "" {
		return transport, nil
	}

	pemData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file %q: %w", caFile, err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool() // fall back to empty pool on unsupported platforms
	}
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("CA file %q: no valid PEM certificates found", caFile)
	}

	tlsCfg := transport.TLSClientConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		tlsCfg = tlsCfg.Clone()
	}
	tlsCfg.RootCAs = pool
	transport.TLSClientConfig = tlsCfg
	return transport, nil
}

// checkRedirect implements the redirect policy:
//   - max maxRedirects hops,
//   - same origin only (scheme + hostname + port must all match),
//   - no HTTPS-to-HTTP downgrade.
//
// Credentials are added per-request in do().  Go strips Authorization on
// host changes; our cross-origin guard ensures they are never forwarded.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("flexibee: too many redirects (max %d)", maxRedirects)
	}

	origin, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("flexibee: invalid base URL: %w", err)
	}

	if origin.Scheme == "https" && req.URL.Scheme == "http" {
		return fmt.Errorf("flexibee: redirect refused: HTTPS→HTTP downgrade to %s",
			req.URL.Redacted())
	}
	if !sameOrigin(origin, req.URL) {
		return fmt.Errorf("flexibee: redirect refused: cross-origin to %s (origin %s)",
			req.URL.Redacted(), origin.Redacted())
	}
	return nil
}

// sameOrigin reports whether a and b share scheme, hostname, and effective port.
func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

// effectivePort returns the explicit port from u, or the default for the scheme.
func effectivePort(u *url.URL) string {
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

// Get performs an authenticated GET request and returns the raw response body.
//
// path is a URL path (and optional query string) relative to cfg.BaseURL.
// Context cancellation is respected.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

// Post performs an authenticated POST request with a JSON body and returns the
// raw response body.  body is sent as application/json.
func (c *Client) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

// do executes a single HTTP request and returns the raw response body.
// Passwords are never logged or included in error messages.
// Response bodies are never logged at the transport layer.
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	rawURL := c.baseURL + path

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, NewConfigError(fmt.Sprintf("building %s request for %q: %v", method, path, err))
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, NewTransportError(fmt.Sprintf("%s %q: %v", method, path, err), err)
		}
		return nil, NewTransportError(fmt.Sprintf("%s %q: %v", method, path, err), err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewTransportError(
			fmt.Sprintf("reading response body from %s %q: %v", method, path, err), err)
	}

	// 2xx: return to caller; ParseAppErrors handles 200-body application errors.
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return respBody, nil
	}

	// NewHTTPError performs HTML detection, JSON message extraction, and
	// status-code-to-ErrorKind mapping.
	return nil, NewHTTPError(resp.StatusCode, respBody)
}

// ── Domain method stubs (pending WP7) ─────────────────────────────────────────

// Ping checks server reachability by fetching /c.json.
func (c *Client) Ping(ctx context.Context) (string, error) {
	body, err := c.Get(ctx, "/c.json")
	if err != nil {
		return "", err
	}
	var env CompaniesEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", NewTransportError("parsing /c.json: "+err.Error(), err)
	}
	return "", nil
}

// ListCompanies returns all companies accessible with the configured credentials.
func (c *Client) ListCompanies(ctx context.Context) ([]Company, error) {
	body, err := c.Get(ctx, "/c.json")
	if err != nil {
		return nil, err
	}
	var env CompaniesEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing /c.json: "+err.Error(), err)
	}
	return env.Companies.Company, nil
}

// ListEvidences returns the evidence catalog for the given company.
func (c *Client) ListEvidences(ctx context.Context, company string) ([]Evidence, error) {
	body, err := c.Get(ctx, "/c/"+company+"/evidence-list.json")
	if err != nil {
		return nil, err
	}
	var env EvidencesEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing evidence-list.json: "+err.Error(), err)
	}
	return env.Evidences.Evidence, nil
}

// ListProperties returns field definitions for the given company/evidence pair.
func (c *Client) ListProperties(ctx context.Context, company, evidence string) ([]Property, error) {
	body, err := c.Get(ctx, "/c/"+company+"/"+evidence+"/properties.json")
	if err != nil {
		return nil, err
	}
	var env PropertiesEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing properties.json: "+err.Error(), err)
	}
	return env.Properties.Property, nil
}

// ListRelations returns relation definitions for the given company/evidence pair.
func (c *Client) ListRelations(ctx context.Context, company, evidence string) ([]Relation, error) {
	body, err := c.Get(ctx, "/c/"+company+"/"+evidence+"/relations.json")
	if err != nil {
		return nil, err
	}
	var env RelationsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing relations.json: "+err.Error(), err)
	}
	return env.Relations.Relation, nil
}

// ListReports returns report definitions for the given company/evidence pair.
func (c *Client) ListReports(ctx context.Context, company, evidence string) ([]Report, error) {
	body, err := c.Get(ctx, "/c/"+company+"/"+evidence+"/reports.json")
	if err != nil {
		return nil, err
	}
	var env ReportsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing reports.json: "+err.Error(), err)
	}
	return env.Reports.Report, nil
}

// QueryRecords executes a bounded record query.
// Returns matched records, optional total count, rendered filter expression, error.
func (c *Client) QueryRecords(ctx context.Context, opts QueryOptions) (
	[]map[string]interface{}, *int, string, error,
) {
	q := &Query{
		Company:  opts.Company,
		Evidence: opts.Evidence,
		Filter:   opts.Filter,
		Fields:   opts.Fields,
		Sort:     opts.Sort,
		Limit:    opts.Limit,
		Start:    opts.Start,
		AddCount: opts.AddCount,
	}
	filterExpr := q.Result().FilterExpr

	postBody, err := q.BuildPostBody()
	if err != nil {
		return nil, nil, filterExpr, err
	}
	body, err := c.Post(ctx, q.PostURL(), postBody)
	if err != nil {
		// Fall back to GET for cases where POST is not accepted.
		body, err = c.Get(ctx, q.BuildURL())
		if err != nil {
			return nil, nil, filterExpr, err
		}
	}

	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, nil, filterExpr, appErr
	}

	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, nil, filterExpr, NewTransportError("parsing query response: "+err.Error(), err)
	}

	var records []map[string]interface{}
	if len(env.Records) > 0 {
		if err := json.Unmarshal(env.Records[0], &records); err != nil {
			return nil, nil, filterExpr, NewTransportError("parsing records array: "+err.Error(), err)
		}
	}

	var total *int
	if opts.AddCount && env.RowCount != "" {
		n := parseRowCount(env.RowCount)
		total = &n
	}
	return records, total, filterExpr, nil
}

// GetRecord fetches a single record by identifier.
// fields restricts the projection; nil means all fields.
func (c *Client) GetRecord(ctx context.Context, company, evidence, identifier string, fields []string) (map[string]interface{}, error) {
	q := &Query{Company: company, Evidence: evidence, Fields: fields}
	path := "/c/" + company + "/" + evidence + "/" + identifier + ".json" +
		buildQueryString(q, false)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing record response: "+err.Error(), err)
	}
	var records []map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &records)
	}
	if len(records) == 0 {
		return nil, NewNotFoundError(fmt.Sprintf("%s/%s: record %q not found", company, evidence, identifier))
	}
	return records[0], nil
}

// GetRelatedRecords fetches records from a named relation on an existing record.
func (c *Client) GetRelatedRecords(ctx context.Context, company, evidence, identifier, relation string, limit, start int) ([]map[string]interface{}, error) {
	q := &Query{Company: company, Evidence: evidence, Limit: limit, Start: start}
	path := "/c/" + company + "/" + evidence + "/" + identifier + "/" + relation + ".json" +
		buildQueryString(q, true)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing related records: "+err.Error(), err)
	}
	var records []map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &records)
	}
	return records, nil
}

// SumRecords calls /$sum.json for a document evidence and returns the totals
// map and the filter expression used.  Returns ErrNotFound when the evidence
// does not support sum operations (FlexiBee returns 404 for non-document evidences).
func (c *Client) SumRecords(ctx context.Context, company, evidence string, filter FilterNode) (map[string]interface{}, string, error) {
	filterExpr := ""
	if filter != nil {
		filterExpr = filter.Build()
	}
	path := "/c/" + company + "/" + evidence + "/$sum.json"
	if filterExpr != "" {
		path = "/c/" + company + "/" + evidence + "/(" + filterPathEscape(filterExpr) + ")/$sum.json"
	}
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, filterExpr, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, filterExpr, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, filterExpr, NewTransportError("parsing sum response: "+err.Error(), err)
	}
	var totals map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &totals)
	}
	return totals, filterExpr, nil
}

// parseRowCount converts a FlexiBee @rowCount string to int.
// Returns 0 on any parse failure.
func parseRowCount(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
