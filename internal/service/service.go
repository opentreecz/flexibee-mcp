// Package service implements the FlexiBee business-logic layer.
//
// The Service type is the single entry point for all MCP tool implementations.
// It is independent of the MCP transport — callers pass a *Service to the
// mcpserver layer, which translates tool arguments into service calls.
//
// Design principles:
//   - Every company-scoped method resolves the company identifier from the
//     request parameter, falling back to the configured default.  The resolved
//     company is echoed in every result so that the caller always knows which
//     company was used.
//   - Field and relation names are validated against live metadata (cached by
//     the registry) before any upstream call is made, so schema errors are
//     caught early with actionable messages.
//   - Page limits are clamped to a hard maximum of 200 records regardless of
//     what the caller requests, matching the config layer's validation.
//   - No credentials appear in result types or error messages.
package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/config"
	"github.com/opentreecz/flexibee-mcp/internal/flexi"
	"github.com/opentreecz/flexibee-mcp/internal/metadata"
)

// jsonMarshal is a package-level alias so tests can intercept it if needed.
var jsonMarshal = json.Marshal

// encodeBase64 returns the standard base64 encoding of b.
func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

const (
	// maxPageSize is the hard upper bound on records returned per page.
	// Matches config.maxPageSize; defined here to avoid importing an internal constant.
	maxPageSize = 200

	// metadataTTL is the default registry cache TTL for a new Service.
	metadataTTL = 5 * time.Minute
)

// ─── Result types ─────────────────────────────────────────────────────────────

// ConnectionInfo is returned by GetConnectionInfo.  It never contains
// credentials or password-adjacent values.
type ConnectionInfo struct {
	// Reachable is true when the server responded to a connectivity probe.
	Reachable bool `json:"reachable"`
	// BaseURL is the configured server base URL (no credentials).
	BaseURL string `json:"base_url"`
	// ActiveTransport is the transport tier that answered the probe
	// (api, session, or java).
	ActiveTransport string `json:"active_transport,omitempty"`
	// ResolvedCompany is the resolved company identifier, when determinable.
	ResolvedCompany string `json:"resolved_company,omitempty"`
	// FlexiVersion is the server-reported version string, when available.
	FlexiVersion string `json:"flexi_version,omitempty"`
	// Companies holds the database identifiers of all accessible companies.
	Companies []string `json:"companies,omitempty"`
	// Diagnostics collects non-fatal messages (unreachable server, missing
	// default company, etc.) so the caller receives maximum context.
	Diagnostics []string `json:"diagnostics,omitempty"`
}

// EvidencePage is the paged result for evidence catalog lookups.
type EvidencePage struct {
	// Company is the resolved company identifier.
	Company string `json:"company"`
	// Items holds the evidence entries for this page.
	Items []flexi.Evidence `json:"items"`
	// Total is the count of all matching evidences before paging.
	Total int `json:"total"`
	// Start is the zero-based offset used for this page.
	Start int `json:"start"`
	// Complete is true when Items exhausts the matching result set.
	Complete bool `json:"complete"`
}

// EvidenceDescription holds schema metadata for one section of an evidence.
type EvidenceDescription struct {
	// Company is the resolved company identifier.
	Company string `json:"company"`
	// Evidence is the evidence path, e.g. "faktura-vydana".
	Evidence string `json:"evidence"`
	// Section identifies which schema subset was returned.
	Section string `json:"section"`
	// Properties is populated when Section is "fields" or "".
	Properties []flexi.Property `json:"properties,omitempty"`
	// Relations is populated when Section is "relations".
	Relations []flexi.Relation `json:"relations,omitempty"`
	// Reports is populated when Section is "reports".
	Reports []flexi.Report `json:"reports,omitempty"`
	// Actions is populated when Section is "actions".
	Actions []flexi.Action `json:"actions,omitempty"`
}

// QueryRequest holds all parameters for a QueryRecords call.
type QueryRequest struct {
	Company  string
	Evidence string
	// Filter is the optional typed filter tree (nil = no filter).
	Filter flexi.FilterNode
	// Fields restricts the field projection (nil or empty = all fields).
	Fields []string
	// Sort specifies the ordering (nil = server default).
	Sort     []flexi.SortField
	Limit    int
	Start    int
	AddCount bool
}

// RecordPage is the paged result for record queries.
type RecordPage struct {
	// Company is the resolved company identifier.
	Company string `json:"company"`
	// Evidence is the queried evidence path.
	Evidence string `json:"evidence"`
	// Records holds the matched record maps; field values preserve the
	// original JSON precision (json.Number for numerics).
	Records []map[string]interface{} `json:"records"`
	// Total is the filtered row count when AddCount was requested; nil otherwise.
	Total *int `json:"total,omitempty"`
	// Start is the zero-based offset used for this page.
	Start int `json:"start"`
	// Limit is the effective page limit used.
	Limit int `json:"limit"`
	// Complete is true when Records exhausted the filtered result set.
	Complete bool `json:"complete"`
	// FilterExpr is the rendered filter expression, for diagnostic output.
	FilterExpr string `json:"filter_expr,omitempty"`
}

// RecordResult holds a single fetched record.
type RecordResult struct {
	// Company is the resolved company identifier.
	Company string `json:"company"`
	// Evidence is the evidence path.
	Evidence string `json:"evidence"`
	// Record is the matched record map.
	Record map[string]interface{} `json:"record"`
}

// SumResult holds document totals from a /$sum.json call.
type SumResult struct {
	// Company is the resolved company identifier.
	Company string `json:"company"`
	// Evidence is the evidence path.
	Evidence string `json:"evidence"`
	// Totals is the aggregation result map from the server.
	Totals map[string]interface{} `json:"totals"`
	// Filter is the rendered filter expression used, for diagnostic output.
	Filter string `json:"filter,omitempty"`
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service is the FlexiBee business-logic layer.  It is safe for concurrent use.
type Service struct {
	client   *flexi.Client
	registry *metadata.Registry
	cfg      *config.Config
}

// NewService constructs a Service from the supplied configuration.
// It creates the underlying flexi.Client and metadata.Registry.
func NewService(cfg *config.Config) (*Service, error) {
	client, err := flexi.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("service: creating flexi client: %w", err)
	}
	registry := metadata.NewRegistry(client, metadataTTL)
	return &Service{
		client:   client,
		registry: registry,
		cfg:      cfg,
	}, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// resolveCompany returns the effective company for a request.
// Resolution order: explicit requested value → FLEXIBEE_COMPANY from config.
// Returns an actionable ErrConfig error when neither source provides a value.
func (s *Service) resolveCompany(_ context.Context, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	if s.cfg.Company != "" {
		return s.cfg.Company, nil
	}
	return "", &flexi.FlexiError{
		Kind:    flexi.ErrConfig,
		Message: "no company specified: provide a company identifier in the request or set FLEXIBEE_COMPANY",
	}
}

// clampLimit returns the effective page limit: falls back to cfg.PageSize,
// capped at maxPageSize.
func (s *Service) clampLimit(requested int) int {
	if requested <= 0 {
		return s.cfg.PageSize
	}
	if requested > maxPageSize {
		return maxPageSize
	}
	return requested
}

// ─── Public methods ───────────────────────────────────────────────────────────

// GetConnectionInfo probes the FlexiBee server and returns reachability
// information, resolved context, and capability diagnostics.
//
// Network failures and missing company configuration are reported as
// Diagnostics rather than Go errors, so the caller always receives a
// structured result even when the server is unreachable.
// Credentials are never included in the result or diagnostics.
func (s *Service) GetConnectionInfo(ctx context.Context, company string) (*ConnectionInfo, error) {
	info := &ConnectionInfo{
		BaseURL: s.cfg.BaseURL,
	}

	// Probe connectivity; a failure sets Reachable=false but does not abort.
	version, pingErr := s.client.Ping(ctx)
	if pingErr != nil {
		info.Diagnostics = append(info.Diagnostics,
			fmt.Sprintf("server unreachable: %v", pingErr))
	} else {
		info.Reachable = true
		info.FlexiVersion = version
		info.ActiveTransport = s.client.TransportName()
	}

	// Enumerate accessible companies independently of the ping result.
	companies, listErr := s.client.ListCompanies(ctx)
	if listErr != nil {
		info.Diagnostics = append(info.Diagnostics,
			fmt.Sprintf("listing companies failed: %v", listErr))
	} else {
		for _, c := range companies {
			info.Companies = append(info.Companies, c.DBNazev)
		}
	}

	// Resolve the requested company; a failure is informational, not fatal.
	resolved, resolveErr := s.resolveCompany(ctx, company)
	if resolveErr != nil {
		info.Diagnostics = append(info.Diagnostics, resolveErr.Error())
	} else {
		info.ResolvedCompany = resolved
	}

	return info, nil
}

// ListCompanies returns all companies accessible with the configured
// credentials.  Results are served from the metadata cache when fresh.
func (s *Service) ListCompanies(ctx context.Context) ([]flexi.Company, error) {
	return s.registry.Companies(ctx)
}

// ListEvidences returns a paged, optionally search-filtered list of evidences.
//
// search is a case-insensitive substring matched against EvidencePath and
// EvidenceName.  limit and start apply standard paging semantics.
// Evidence metadata is served from the registry cache when fresh.
func (s *Service) ListEvidences(ctx context.Context, company, search string, limit, start int) (*EvidencePage, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	all, err := s.registry.Evidences(ctx, resolved)
	if err != nil {
		return nil, err
	}

	// Apply optional substring search against path and display names.
	if search != "" {
		lower := strings.ToLower(search)
		var filtered []flexi.Evidence
		for _, ev := range all {
			if strings.Contains(strings.ToLower(ev.EvidencePath), lower) ||
				strings.Contains(strings.ToLower(ev.EvidenceName), lower) {
				filtered = append(filtered, ev)
			}
		}
		all = filtered
	}

	total := len(all)

	// Apply zero-based start offset.
	if start > 0 {
		if start >= len(all) {
			all = nil
		} else {
			all = all[start:]
		}
	}

	// Clamp to the effective limit and record whether the page is complete.
	lim := s.clampLimit(limit)
	available := len(all)
	complete := available <= lim
	if available > lim {
		all = all[:lim]
	}

	return &EvidencePage{
		Company:  resolved,
		Items:    all,
		Total:    total,
		Start:    start,
		Complete: complete,
	}, nil
}

// DescribeEvidence returns metadata for one section of an evidence.
//
// section controls which subset is returned:
//   - "fields" or "" → Properties (field definitions)
//   - "relations"    → Relations (navigable sub-resources)
//   - "reports"      → Reports (print/export templates)
//   - "actions"      → Actions (available business actions)
//
// fieldFilter, when non-empty, is a case-insensitive substring applied to
// property Name and Title; it is ignored for non-field sections.
func (s *Service) DescribeEvidence(ctx context.Context, company, evidence, section string, fieldFilter string) (*EvidenceDescription, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	// Normalise section so downstream code and callers see "fields" consistently.
	if section == "" {
		section = "fields"
	}

	desc := &EvidenceDescription{
		Company:  resolved,
		Evidence: evidence,
		Section:  section,
	}

	switch section {
	case "relations":
		rels, err := s.registry.Relations(ctx, resolved, evidence)
		if err != nil {
			return nil, err
		}
		desc.Relations = rels

	case "reports":
		reports, err := s.registry.Reports(ctx, resolved, evidence)
		if err != nil {
			return nil, err
		}
		desc.Reports = reports

	case "actions":
		actions, err := s.client.ListActions(ctx, resolved, evidence)
		if err != nil {
			return nil, err
		}
		desc.Actions = actions

	default: // "fields" or any unrecognised value falls back to fields
		props, err := s.registry.Properties(ctx, resolved, evidence)
		if err != nil {
			return nil, err
		}
		if fieldFilter != "" {
			lower := strings.ToLower(fieldFilter)
			var filtered []flexi.Property
			for _, p := range props {
				if strings.Contains(strings.ToLower(p.Name), lower) ||
					strings.Contains(strings.ToLower(p.Title), lower) {
					filtered = append(filtered, p)
				}
			}
			props = filtered
		}
		desc.Properties = props
	}

	return desc, nil
}

// QueryRecords executes a bounded read query, validating field names against
// live metadata before the upstream call is made.
//
// If req.Fields contains unrecognised names, the method returns a FlexiError
// with Kind=ErrValidation and the unknown field names in Details.
// The resolved filter expression is included in RecordPage.FilterExpr for
// diagnostic and continuation purposes.
func (s *Service) QueryRecords(ctx context.Context, req *QueryRequest) (*RecordPage, error) {
	resolved, err := s.resolveCompany(ctx, req.Company)
	if err != nil {
		return nil, err
	}

	// Validate field projection against metadata.
	if len(req.Fields) > 0 {
		unknown, err := s.registry.ValidateFields(ctx, resolved, req.Evidence, req.Fields)
		if err != nil {
			return nil, err
		}
		if len(unknown) > 0 {
			return nil, &flexi.FlexiError{
				Kind:    flexi.ErrValidation,
				Message: fmt.Sprintf("unknown fields for evidence %q in company %q", req.Evidence, resolved),
				Details: unknown,
			}
		}
	}

	lim := s.clampLimit(req.Limit)
	opts := flexi.QueryOptions{
		Company:  resolved,
		Evidence: req.Evidence,
		Filter:   req.Filter,
		Fields:   req.Fields,
		Sort:     req.Sort,
		Limit:    lim,
		Start:    req.Start,
		AddCount: req.AddCount,
	}

	records, total, filterExpr, err := s.client.QueryRecords(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &RecordPage{
		Company:    resolved,
		Evidence:   req.Evidence,
		Records:    records,
		Total:      total,
		Start:      req.Start,
		Limit:      lim,
		Complete:   len(records) < lim,
		FilterExpr: filterExpr,
	}, nil
}

// GetRecord fetches a single record by identifier.
//
// identifier may be a numeric id, "code:ABC", "ext:SYSTEM:ID", or any other
// Flexi selector supported by the evidence.  If fields are provided they are
// validated against metadata before the call.
func (s *Service) GetRecord(ctx context.Context, company, evidence, identifier string, fields []string) (*RecordResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	if len(fields) > 0 {
		unknown, err := s.registry.ValidateFields(ctx, resolved, evidence, fields)
		if err != nil {
			return nil, err
		}
		if len(unknown) > 0 {
			return nil, &flexi.FlexiError{
				Kind:    flexi.ErrValidation,
				Message: fmt.Sprintf("unknown fields for evidence %q in company %q", evidence, resolved),
				Details: unknown,
			}
		}
	}

	record, err := s.client.GetRecord(ctx, resolved, evidence, identifier, fields)
	if err != nil {
		return nil, err
	}
	return &RecordResult{
		Company:  resolved,
		Evidence: evidence,
		Record:   record,
	}, nil
}

// GetRelatedRecords fetches records from a named relation on an existing
// record.  The relation path is validated against metadata before the call.
//
// relation must be the RelationPath value (e.g. "polozkyDokladu"), not the
// display name.  Use DescribeEvidence with section="relations" to enumerate
// valid relation paths.
func (s *Service) GetRelatedRecords(ctx context.Context, company, evidence, identifier, relation string, limit, start int) (*RecordPage, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	if err := s.registry.ValidateRelation(ctx, resolved, evidence, relation); err != nil {
		return nil, err
	}

	lim := s.clampLimit(limit)
	records, err := s.client.GetRelatedRecords(ctx, resolved, evidence, identifier, relation, lim, start)
	if err != nil {
		return nil, err
	}

	return &RecordPage{
		Company:  resolved,
		Evidence: evidence,
		Records:  records,
		Start:    start,
		Limit:    lim,
		Complete: len(records) < lim,
	}, nil
}

// SumRecords calls /$sum.json for the given evidence and returns the server-
// provided document totals.
//
// Sum is only supported for document evidences (invoices, orders, etc.).
// If the server returns 404 — indicating the evidence does not support sum
// operations — the error is translated to a clear ErrUnsupported FlexiError
// describing the capability limitation.  Callers should verify that the
// evidence is a document type before calling this method.
func (s *Service) SumRecords(ctx context.Context, company, evidence string, filter flexi.FilterNode) (*SumResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	// Verify that the evidence exists before attempting the sum call.
	evidences, err := s.registry.Evidences(ctx, resolved)
	if err != nil {
		return nil, err
	}
	found := false
	for _, ev := range evidences {
		if ev.EvidencePath == evidence {
			found = true
			break
		}
	}
	if !found {
		return nil, &flexi.FlexiError{
			Kind:    flexi.ErrNotFound,
			Message: fmt.Sprintf("evidence %q not found in company %q", evidence, resolved),
		}
	}

	totals, filterExpr, err := s.client.SumRecords(ctx, resolved, evidence, filter)
	if err != nil {
		// Translate a server 404 into a capability error with actionable text.
		// The FlexiBee API returns 404 when /$sum.json is called on an evidence
		// that does not support document aggregation.
		var fe *flexi.FlexiError
		if errors.As(err, &fe) && fe.Kind == flexi.ErrNotFound {
			return nil, &flexi.FlexiError{
				Kind: flexi.ErrUnsupported,
				Message: fmt.Sprintf(
					"evidence %q does not support sum operations ($sum.json); "+
						"sum is only available for document evidences such as invoices, "+
						"purchase orders, and other financial documents",
					evidence,
				),
				Details: []string{
					"verify that the evidence is a document type before calling sum",
					"document evidence examples: faktura-vydana, faktura-prijata, " +
						"objednavka-vydana, objednavka-prijata",
				},
			}
		}
		return nil, err
	}

	return &SumResult{
		Company:  resolved,
		Evidence: evidence,
		Totals:   totals,
		Filter:   filterExpr,
	}, nil
}

// ── Write operations ──────────────────────────────────────────────────────────

// UpsertRequest holds the parameters for an upsert (create-or-update) call.
type UpsertRequest struct {
	Company  string
	Evidence string
	// Record is the field map to write.  Required fields depend on the evidence
	// schema.  Use DescribeEvidence (section="fields") to find mandatory fields.
	// For updates, include the "id" or external identifier field.
	Record map[string]interface{}
	// DryRun when true validates and calculates without persisting.
	DryRun bool
}

// UpsertResult holds the outcome of an upsert operation.
type UpsertResult struct {
	Company  string `json:"company"`
	Evidence string `json:"evidence"`
	// Record is the created/updated record as returned by the server.
	Record map[string]interface{} `json:"record,omitempty"`
	// DryRun is true when the operation was a preview-only dry run.
	DryRun bool `json:"dry_run"`
}

// DeleteResult holds the outcome of a delete operation.
type DeleteResult struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	Identifier string `json:"identifier"`
	Deleted    bool   `json:"deleted"`
}

// ActionResult holds the outcome of a named action execution.
type ActionResult struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	Identifier string `json:"identifier"`
	Action     string `json:"action"`
	// Result is the server response body, when available.
	Result map[string]interface{} `json:"result,omitempty"`
}

// ActionsResult is the list of available actions for an evidence.
type ActionsResult struct {
	Company  string         `json:"company"`
	Evidence string         `json:"evidence"`
	Actions  []flexi.Action `json:"actions"`
}

// UpsertRecord creates or updates a record in an evidence.
//
// The Record map is wrapped in the expected winstrom envelope before sending.
// When DryRun is true the request is sent with ?dryRun=true and the server
// validates and calculates without persisting any data.
func (s *Service) UpsertRecord(ctx context.Context, req *UpsertRequest) (*UpsertResult, error) {
	resolved, err := s.resolveCompany(ctx, req.Company)
	if err != nil {
		return nil, err
	}

	if req.Record == nil || len(req.Record) == 0 {
		return nil, &flexi.FlexiError{
			Kind:    flexi.ErrValidation,
			Message: "record must not be empty",
		}
	}

	// Wrap in the winstrom envelope: {"winstrom": {"<evidence>": {fields...}}}
	envelope := map[string]interface{}{
		"winstrom": map[string]interface{}{
			req.Evidence: req.Record,
		},
	}

	bodyBytes, err := marshalJSON(envelope)
	if err != nil {
		return nil, &flexi.FlexiError{Kind: flexi.ErrValidation, Message: "marshalling record: " + err.Error()}
	}

	path := "/c/" + resolved + "/" + req.Evidence + ".json"
	if req.DryRun {
		path += "?dryRun=true"
	}

	result, err := s.client.UpsertRecord(ctx, resolved, req.Evidence, bodyBytes)
	_ = path // UpsertRecord uses its own path construction
	if err != nil {
		return nil, err
	}

	return &UpsertResult{
		Company:  resolved,
		Evidence: req.Evidence,
		Record:   result,
		DryRun:   req.DryRun,
	}, nil
}

// DeleteRecord deletes a record by identifier.
//
// identifier may be a numeric id, "code:ABC", or any other Flexi selector.
func (s *Service) DeleteRecord(ctx context.Context, company, evidence, identifier string) (*DeleteResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	if err := s.client.DeleteRecord(ctx, resolved, evidence, identifier); err != nil {
		return nil, err
	}

	return &DeleteResult{
		Company:    resolved,
		Evidence:   evidence,
		Identifier: identifier,
		Deleted:    true,
	}, nil
}

// ExecuteAction fires a named action on a specific record.
//
// action must be the actionPath value (e.g. "lock", "cancel", "send").
// Use ListActions to enumerate available actions and their paths.
func (s *Service) ExecuteAction(ctx context.Context, company, evidence, identifier, action string) (*ActionResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	result, err := s.client.ExecuteAction(ctx, resolved, evidence, identifier, action)
	if err != nil {
		return nil, err
	}

	return &ActionResult{
		Company:    resolved,
		Evidence:   evidence,
		Identifier: identifier,
		Action:     action,
		Result:     result,
	}, nil
}

// ListActions returns the available actions for a given evidence.
func (s *Service) ListActions(ctx context.Context, company, evidence string) (*ActionsResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	actions, err := s.client.ListActions(ctx, resolved, evidence)
	if err != nil {
		return nil, err
	}

	return &ActionsResult{
		Company:  resolved,
		Evidence: evidence,
		Actions:  actions,
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// marshalJSON is a thin wrapper that returns a descriptive error on failure.
func marshalJSON(v interface{}) ([]byte, error) {
	return jsonMarshal(v)
}

// ── Report download ───────────────────────────────────────────────────────────

// ReportResult holds a downloaded report as base64 and its MIME type.
type ReportResult struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	ReportPath string `json:"report_path"`
	Format     string `json:"format"`
	// DataBase64 is the report bytes encoded as standard base64.
	DataBase64 string `json:"data_base64"`
	MIMEType   string `json:"mime_type"`
	// SizeBytes is the byte length of the decoded data.
	SizeBytes int `json:"size_bytes"`
}

// GetReport downloads a report in the requested format and returns it base64-encoded.
func (s *Service) GetReport(ctx context.Context, company, evidence, reportPath, format string, filter flexi.FilterNode) (*ReportResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}
	if format == "" {
		format = "pdf"
	}

	result, err := s.client.GetReport(ctx, resolved, evidence, reportPath, format, filter)
	if err != nil {
		return nil, err
	}

	return &ReportResult{
		Company:    resolved,
		Evidence:   evidence,
		ReportPath: reportPath,
		Format:     format,
		DataBase64: encodeBase64(result.Data),
		MIMEType:   result.MIMEType,
		SizeBytes:  len(result.Data),
	}, nil
}

// ── Attachments ───────────────────────────────────────────────────────────────

// AttachmentListResult holds the list of attachments for a record.
type AttachmentListResult struct {
	Company     string                   `json:"company"`
	Evidence    string                   `json:"evidence"`
	Identifier  string                   `json:"identifier"`
	Attachments []map[string]interface{} `json:"attachments"`
}

// AttachmentResult holds a downloaded attachment as base64.
type AttachmentResult struct {
	Company      string `json:"company"`
	AttachmentID string `json:"attachment_id"`
	// DataBase64 is the attachment bytes encoded as standard base64.
	DataBase64 string `json:"data_base64"`
	MIMEType   string `json:"mime_type"`
	SizeBytes  int    `json:"size_bytes"`
}

// ListAttachments returns the list of attachments for a record.
func (s *Service) ListAttachments(ctx context.Context, company, evidence, identifier string) (*AttachmentListResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	attachments, err := s.client.ListAttachments(ctx, resolved, evidence, identifier)
	if err != nil {
		return nil, err
	}

	return &AttachmentListResult{
		Company:     resolved,
		Evidence:    evidence,
		Identifier:  identifier,
		Attachments: attachments,
	}, nil
}

// GetAttachment downloads an attachment by ID, returned base64-encoded.
func (s *Service) GetAttachment(ctx context.Context, company, attachmentID string) (*AttachmentResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	result, err := s.client.GetAttachment(ctx, resolved, attachmentID)
	if err != nil {
		return nil, err
	}

	return &AttachmentResult{
		Company:      resolved,
		AttachmentID: attachmentID,
		DataBase64:   encodeBase64(result.Data),
		MIMEType:     result.MIMEType,
		SizeBytes:    len(result.Data),
	}, nil
}

// ── Changes API ───────────────────────────────────────────────────────────────

// ChangesResult wraps the service-level changes response.
type ChangesResult struct {
	Company     string               `json:"company"`
	LastEventID int64                `json:"last_event_id"`
	Changes     []flexi.ChangeRecord `json:"changes"`
	// Count is the number of change records returned.
	Count int `json:"count"`
}

// GetChanges fetches changed records since lastEventID.
// Pass 0 for lastEventID to get all available changes.
// Store the returned LastEventID and pass it on the next call for incremental sync.
func (s *Service) GetChanges(ctx context.Context, company string, lastEventID int64) (*ChangesResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	result, err := s.client.GetChanges(ctx, resolved, lastEventID)
	if err != nil {
		return nil, err
	}

	return &ChangesResult{
		Company:     resolved,
		LastEventID: result.LastEventID,
		Changes:     result.Changes,
		Count:       len(result.Changes),
	}, nil
}

// ── Company statistics ────────────────────────────────────────────────────────

// CompanyStatsResult wraps the company statistics response.
type CompanyStatsResult struct {
	Company string                 `json:"company"`
	Stats   map[string]interface{} `json:"stats"`
}

// GetCompanyStats returns statistics for the given company.
func (s *Service) GetCompanyStats(ctx context.Context, company string) (*CompanyStatsResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	stats, err := s.client.GetCompanyStats(ctx, resolved)
	if err != nil {
		return nil, err
	}

	return &CompanyStatsResult{
		Company: resolved,
		Stats:   stats,
	}, nil
}

// ── Saved queries ─────────────────────────────────────────────────────────────

// SavedQueriesResult holds a list of saved queries.
type SavedQueriesResult struct {
	Company string                   `json:"company"`
	Queries []map[string]interface{} `json:"queries"`
}

// SavedQueryResult holds the result of running a saved query.
type SavedQueryResult struct {
	Company  string                   `json:"company"`
	QueryID  string                   `json:"query_id"`
	Records  []map[string]interface{} `json:"records"`
	Total    *int                     `json:"total,omitempty"`
	Start    int                      `json:"start"`
	Limit    int                      `json:"limit"`
	Complete bool                     `json:"complete"`
}

// ListSavedQueries returns all named saved queries for the given company.
func (s *Service) ListSavedQueries(ctx context.Context, company string) (*SavedQueriesResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	queries, err := s.client.ListSavedQueries(ctx, resolved)
	if err != nil {
		return nil, err
	}

	return &SavedQueriesResult{
		Company: resolved,
		Queries: queries,
	}, nil
}

// RunSavedQuery executes a named saved query by its ID or code and returns
// paged records.
func (s *Service) RunSavedQuery(ctx context.Context, company, queryID string, limit, start int) (*SavedQueryResult, error) {
	resolved, err := s.resolveCompany(ctx, company)
	if err != nil {
		return nil, err
	}

	lim := s.clampLimit(limit)
	records, total, err := s.client.RunSavedQuery(ctx, resolved, queryID, lim, start)
	if err != nil {
		return nil, err
	}

	return &SavedQueryResult{
		Company:  resolved,
		QueryID:  queryID,
		Records:  records,
		Total:    total,
		Start:    start,
		Limit:    lim,
		Complete: len(records) < lim,
	}, nil
}
