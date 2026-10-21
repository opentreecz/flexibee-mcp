// Package flexi provides the FlexiBee HTTP client, typed filter tree, and
// query builder used by the MCP tool layer.
//
// Package layout:
//
//   - client.go          – FilterNode, RawFilter, SortField, QueryOptions, Client
//   - transport.go       – Transport interface, AutoTransport, shared HTTP helpers
//   - transport_api.go   – API (Basic Auth) transport
//   - transport_session.go – v2 Session transport (no licence required)
//   - transport_java.go  – Java subprocess transport (native client mimic)
//   - query.go           – filter implementations, Query, QueryResult, URL/body builders
//   - envelopes.go       – response envelope types (Company, Evidence, Property, …)
//   - errors.go          – FlexiError, ErrorKind, constructors, ParseAppErrors
package flexi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/opentreecz/flexibee-mcp/internal/config"
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

// Client is an authenticated FlexiBee REST API client.
//
// It uses a Transport to perform HTTP communication and handles response
// envelope parsing and business logic.  The active transport is selected
// automatically (api → session → java) based on FLEXIBEE_AUTH_MODE.
//
// A single Client encapsulates one account's credentials.  Credentials are
// applied by the underlying Transport and never written to any log output.
type Client struct {
	transport Transport
	cfg       *config.Config
}

// NewClient constructs a Client from the validated configuration.
// It creates an AutoTransport that tries API auth first, falling back to
// session auth on HTTP 402, and finally to the Java subprocess transport.
func NewClient(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, NewConfigError("config must not be nil")
	}
	if cfg.BaseURL == "" {
		return nil, NewConfigError("base URL is required")
	}

	tr, err := NewAutoTransport(cfg)
	if err != nil {
		return nil, fmt.Errorf("flexi.NewClient: %w", err)
	}

	return &Client{
		transport: tr,
		cfg:       cfg,
	}, nil
}

// NewClientWithTransport constructs a Client with an explicitly provided
// Transport.  Used in tests and for manual transport overrides.
func NewClientWithTransport(tr Transport, cfg *config.Config) *Client {
	return &Client{transport: tr, cfg: cfg}
}

// TransportName returns the name of the active transport.
func (c *Client) TransportName() string {
	return c.transport.Name()
}

// ── HTTP primitives ───────────────────────────────────────────────────────────

// Get performs an authenticated GET request and returns the raw response body.
// path is relative to cfg.BaseURL.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	return c.transport.Get(ctx, path)
}

// Post performs an authenticated POST request with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.transport.Post(ctx, path, body)
}

// Put performs an authenticated PUT request with a JSON body.
func (c *Client) Put(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.transport.Put(ctx, path, body)
}

// Delete performs an authenticated DELETE request.
func (c *Client) Delete(ctx context.Context, path string) ([]byte, error) {
	return c.transport.Delete(ctx, path)
}

// ── Domain methods ────────────────────────────────────────────────────────────

// Ping checks server reachability by fetching /c.json.
// Returns the server version string if available, or an empty string.
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

// ListActions returns action definitions for the given company/evidence pair.
func (c *Client) ListActions(ctx context.Context, company, evidence string) ([]Action, error) {
	body, err := c.Get(ctx, "/c/"+company+"/"+evidence+"/actions.json")
	if err != nil {
		return nil, err
	}
	var env ActionsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing actions.json: "+err.Error(), err)
	}
	return env.Actions.Action, nil
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

// ── Write operations ──────────────────────────────────────────────────────────

// ── Write operations ──────────────────────────────────────────────────────────

// UpsertRecord creates or updates a record in an evidence.
// The body must be a valid winstrom-wrapped JSON object:
//
//	{"winstrom": {"<evidence>": {<fields>}}}
func (c *Client) UpsertRecord(ctx context.Context, company, evidence string, body []byte) (map[string]interface{}, error) {
	path := "/c/" + company + "/" + evidence + ".json"
	respBody, err := c.Put(ctx, path, body)
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(respBody); appErr != nil {
		return nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return nil, NewTransportError("parsing upsert response: "+err.Error(), err)
	}
	var records []map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &records)
	}
	if len(records) == 0 {
		return map[string]interface{}{}, nil
	}
	return records[0], nil
}

// DeleteRecord deletes a record by identifier.
func (c *Client) DeleteRecord(ctx context.Context, company, evidence, identifier string) error {
	path := "/c/" + company + "/" + evidence + "/" + identifier + ".json"
	body, err := c.Delete(ctx, path)
	if err != nil {
		return err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return appErr
	}
	return nil
}

// ExecuteAction fires a named action on a record.
func (c *Client) ExecuteAction(ctx context.Context, company, evidence, identifier, action string) (map[string]interface{}, error) {
	path := "/c/" + company + "/" + evidence + "/" + identifier + "/a/" + action + ".json"
	body, err := c.Post(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing action response: "+err.Error(), err)
	}
	var records []map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &records)
	}
	if len(records) == 0 {
		return map[string]interface{}{}, nil
	}
	return records[0], nil
}

// ── Binary / document downloads ───────────────────────────────────────────────

// BinaryResult holds a raw byte payload and its MIME type.
type BinaryResult struct {
	Data     []byte
	MIMEType string
}

// GetReport downloads a report in the requested format.
// format should be one of: pdf, xlsx, xls, csv, xml, isdoc, isdocx.
// reportPath is the reportPath value from ListReports (e.g. "faktura").
// filter optionally restricts records included in the report.
func (c *Client) GetReport(ctx context.Context, company, evidence, reportPath, format string, filter FilterNode) (*BinaryResult, error) {
	path := "/c/" + company + "/" + evidence + "/" + reportPath + "." + format
	if filter != nil {
		expr := filter.Build()
		if expr != "" {
			path = "/c/" + company + "/" + evidence + "/(" + filterPathEscape(expr) + ")/" + reportPath + "." + format
		}
	}
	body, err := c.transport.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	return &BinaryResult{
		Data:     body,
		MIMEType: mimeForFormat(format),
	}, nil
}

// ListAttachments returns the attachment records for a given evidence record.
// FlexiBee stores attachments in the "priloha" sub-evidence.
func (c *Client) ListAttachments(ctx context.Context, company, evidence, identifier string) ([]map[string]interface{}, error) {
	path := "/c/" + company + "/" + evidence + "/" + identifier + "/prilohy.json"
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing attachments: "+err.Error(), err)
	}
	var records []map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &records)
	}
	return records, nil
}

// GetAttachment downloads the binary content of an attachment.
// attachmentID is the numeric id of the attachment record in "priloha" evidence.
func (c *Client) GetAttachment(ctx context.Context, company, attachmentID string) (*BinaryResult, error) {
	// FlexiBee attachment download: GET /c/{co}/priloha/{id}/content
	path := "/c/" + company + "/priloha/" + attachmentID + "/content"
	body, err := c.transport.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	// MIME type is guessed from filename if available; default to octet-stream.
	return &BinaryResult{
		Data:     body,
		MIMEType: "application/octet-stream",
	}, nil
}

// ── Changes API ───────────────────────────────────────────────────────────────

// ChangeRecord represents one record change from the FlexiBee Changes API.
type ChangeRecord struct {
	ID        string `json:"id"`
	Evidence  string `json:"evidence"`
	Timestamp string `json:"timestamp"`
	Method    string `json:"method"` // INSERT, UPDATE, DELETE
}

// ChangesResult holds the Changes API response.
type ChangesResult struct {
	LastEventID int64          `json:"last_event_id"`
	Changes     []ChangeRecord `json:"changes"`
}

// GetChanges fetches records changed since lastEventID from the Changes API.
// Returns the changes and the new lastEventID to use on the next call.
func (c *Client) GetChanges(ctx context.Context, company string, lastEventID int64) (*ChangesResult, error) {
	path := fmt.Sprintf("/c/%s/changes.json?start=%d&limit=200", company, lastEventID)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, appErr
	}
	// Parse the changes envelope.
	var env struct {
		Winstrom struct {
			LastEventID json.Number `json:"@lastEventId"`
			Changes     json.RawMessage
		} `json:"winstrom"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing changes response: "+err.Error(), err)
	}
	lastID, _ := env.Winstrom.LastEventID.Int64()

	// Parse changes from the WinstromEnvelope.
	var outerEnv WinstromEnvelope
	_ = json.Unmarshal(body, &outerEnv)
	var changes []ChangeRecord
	for _, raw := range outerEnv.Records {
		_ = json.Unmarshal(raw, &changes)
	}

	return &ChangesResult{
		LastEventID: lastID,
		Changes:     changes,
	}, nil
}

// ── Company statistics ────────────────────────────────────────────────────────

// GetCompanyStats returns statistics about the given company.
func (c *Client) GetCompanyStats(ctx context.Context, company string) (map[string]interface{}, error) {
	body, err := c.Get(ctx, "/c/"+company+"/statistics.json")
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing statistics response: "+err.Error(), err)
	}
	var stats map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &stats)
	}
	return stats, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// mimeForFormat maps a FlexiBee format suffix to a MIME type.
func mimeForFormat(format string) string {
	switch format {
	case "pdf":
		return "application/pdf"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "xls":
		return "application/vnd.ms-excel"
	case "csv":
		return "text/csv"
	case "xml":
		return "application/xml"
	case "isdoc", "isdocx":
		return "application/x-isdoc"
	case "json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// ── Saved queries ─────────────────────────────────────────────────────────────

// ListSavedQueries returns all named saved queries for the given company.
// These are stored in the "uzivatelsky-dotaz" evidence.
func (c *Client) ListSavedQueries(ctx context.Context, company string) ([]map[string]interface{}, error) {
	body, err := c.Get(ctx, "/c/"+company+"/uzivatelsky-dotaz.json")
	if err != nil {
		return nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, NewTransportError("parsing saved queries: "+err.Error(), err)
	}
	var queries []map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &queries)
	}
	return queries, nil
}

// RunSavedQuery executes a named saved query by its ID or code.
// Returns records, optional total count, or error.
func (c *Client) RunSavedQuery(ctx context.Context, company, queryID string, limit, start int) (
	[]map[string]interface{}, *int, error,
) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	path := fmt.Sprintf("/c/%s/uzivatelsky-dotaz/%s/call.json?limit=%d&start=%d&add-row-count=true",
		company, queryID, limit, start)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	if appErr := ParseAppErrors(body); appErr != nil {
		return nil, nil, appErr
	}
	var env WinstromEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, nil, NewTransportError("parsing saved query result: "+err.Error(), err)
	}
	var records []map[string]interface{}
	if len(env.Records) > 0 {
		_ = json.Unmarshal(env.Records[0], &records)
	}
	var total *int
	if env.RowCount != "" {
		n := parseRowCount(env.RowCount)
		total = &n
	}
	return records, total, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
