// Package mcpserver wires the FlexiBee service layer to the official
// Model Context Protocol Go SDK (github.com/modelcontextprotocol/go-sdk).
//
// Phase 1 surface:
//   - 9 read-only tools
//   - 3 prompts (review_receivables, prepare_invoice, inspect_inventory)
//   - 6 guide resources  (flexi://guides/…)
//
// All log output goes to stderr; stdout is reserved for the MCP stdio
// transport.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opentreecz/flexibee-mcp/internal/flexi"
	"github.com/opentreecz/flexibee-mcp/internal/observability"
	"github.com/opentreecz/flexibee-mcp/internal/service"
)

// ─── Server ──────────────────────────────────────────────────────────────────

// Server wraps an mcp.Server and the FlexiBee service layer.
type Server struct {
	svc    *service.Service
	server *mcp.Server
	log    *observability.Logger
}

// New creates and configures the MCP server, registering all tools, prompts,
// and resources.
func New(svc *service.Service, version string) *Server {
	s := &Server{
		svc:    svc,
		server: mcp.NewServer("flexibee-mcp", version, nil),
		log:    observability.DefaultLogger,
	}
	s.registerTools()
	s.registerWriteTools()
	s.registerDocumentTools()
	s.registerPrompts()
	s.registerResources()
	return s
}

// Serve runs the server over the stdio MCP transport and blocks until the
// client closes the connection or ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	s.log.Info("mcpserver: starting stdio transport")
	return s.server.Run(ctx, mcp.NewStdioTransport())
}

// ─── Shared result helpers ────────────────────────────────────────────────────

// toolText wraps a plain string in a successful CallToolResult.
func toolText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// toolError wraps an error message in a CallToolResult flagged as IsError.
func toolError(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
		IsError: true,
	}
}

// toolJSON marshals v to JSON and returns it as a successful CallToolResult.
// Marshalling errors are themselves returned as IsError results (not Go errors)
// so the LLM can observe them.
func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError(fmt.Errorf("marshal result: %w", err)), nil
	}
	return toolText(string(b)), nil
}

// readOnly returns a *ToolAnnotations that marks a tool as read-only.
func readOnly() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true}
}

// ─── Tool input structs ───────────────────────────────────────────────────────
//
// github.com/modelcontextprotocol/go-sdk uses jsonschema.For[In]() to infer
// the JSON schema from the struct.  Fields without "omitempty" become required;
// fields with "omitempty" are optional.

type getConnectionInfoArgs struct {
	// Company is optional; falls back to the default from config.
	Company string `json:"company,omitempty"`
}

type listCompaniesArgs struct{}

type listEvidencesArgs struct {
	Company string `json:"company"`
	Search  string `json:"search,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Start   int    `json:"start,omitempty"`
}

type describeEvidenceArgs struct {
	Company     string `json:"company"`
	Evidence    string `json:"evidence"`
	Section     string `json:"section,omitempty"`
	FieldFilter string `json:"field_filter,omitempty"`
}

type queryRecordsArgs struct {
	Company  string   `json:"company"`
	Evidence string   `json:"evidence"`
	Filter   string   `json:"filter,omitempty"`
	Fields   []string `json:"fields,omitempty"`
	Sort     string   `json:"sort,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Start    int      `json:"start,omitempty"`
	AddCount bool     `json:"add_count,omitempty"`
}

type getRecordArgs struct {
	Company    string   `json:"company"`
	Evidence   string   `json:"evidence"`
	Identifier string   `json:"identifier"`
	Fields     []string `json:"fields,omitempty"`
}

type getRelatedRecordsArgs struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	Identifier string `json:"identifier"`
	Relation   string `json:"relation"`
	Limit      int    `json:"limit,omitempty"`
	Start      int    `json:"start,omitempty"`
}

type sumRecordsArgs struct {
	Company  string `json:"company"`
	Evidence string `json:"evidence"`
	Filter   string `json:"filter,omitempty"`
}

type getGuideArgs struct {
	Topic string `json:"topic"`
}

// ─── Tool registration ────────────────────────────────────────────────────────

const defaultLimit = 50

func (s *Server) registerTools() {
	tools := []*mcp.ServerTool{
		s.toolGetConnectionInfo(),
		s.toolListCompanies(),
		s.toolListEvidences(),
		s.toolDescribeEvidence(),
		s.toolQueryRecords(),
		s.toolGetRecord(),
		s.toolGetRelatedRecords(),
		s.toolSumRecords(),
		s.toolGetGuide(),
	}
	s.server.AddTools(tools...)
}

// ── flexi_get_connection_info ─────────────────────────────────────────────────

func (s *Server) toolGetConnectionInfo() *mcp.ServerTool {
	t := mcp.NewServerTool[getConnectionInfoArgs, any](
		"flexi_get_connection_info",
		"Returns connection status, the resolved company identifier, and server "+
			"diagnostics. Never exposes passwords or auth tokens.",
		s.handleGetConnectionInfo,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier. Omit to use the default "+
					"company configured via FLEXIBEE_COMPANY."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetConnectionInfo(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getConnectionInfoArgs],
) (*mcp.CallToolResult, error) {
	result, err := s.svc.GetConnectionInfo(ctx, params.Arguments.Company)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_list_companies ──────────────────────────────────────────────────────

func (s *Server) toolListCompanies() *mcp.ServerTool {
	t := mcp.NewServerTool[listCompaniesArgs, any](
		"flexi_list_companies",
		"Lists all FlexiBee companies available on the configured server, "+
			"returning each company's db identifier and display name.",
		s.handleListCompanies,
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleListCompanies(
	ctx context.Context,
	_ *mcp.ServerSession,
	_ *mcp.CallToolParamsFor[listCompaniesArgs],
) (*mcp.CallToolResult, error) {
	companies, err := s.svc.ListCompanies(ctx)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(companies)
}

// ── flexi_list_evidences ──────────────────────────────────────────────────────

func (s *Server) toolListEvidences() *mcp.ServerTool {
	t := mcp.NewServerTool[listEvidencesArgs, any](
		"flexi_list_evidences",
		"Returns a compact catalog of evidence types (data tables) available in "+
			"the given company.  Use `search` to filter by name or key.",
		s.handleListEvidences,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("search",
				mcp.Description("Optional substring filter applied to evidence name or key."),
			),
			mcp.Property("limit",
				mcp.Description("Maximum items to return (1–200, default 50)."),
			),
			mcp.Property("start",
				mcp.Description("Zero-based offset for pagination."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleListEvidences(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[listEvidencesArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	if a.Limit <= 0 {
		a.Limit = defaultLimit
	}
	if a.Limit > 200 {
		a.Limit = 200
	}
	result, err := s.svc.ListEvidences(ctx, a.Company, a.Search, a.Limit, a.Start)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_describe_evidence ───────────────────────────────────────────────────

func (s *Server) toolDescribeEvidence() *mcp.ServerTool {
	t := mcp.NewServerTool[describeEvidenceArgs, any](
		"flexi_describe_evidence",
		"Returns schema information for a single evidence: field definitions, "+
			"relations, or available reports. Use `section` to select the subset.",
		s.handleDescribeEvidence,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Evidence key, e.g. \"vydane-faktury\" or \"adresar\"."),
				mcp.Required(true),
			),
			mcp.Property("section",
				mcp.Description("Schema section to return: \"fields\" (default), "+
					"\"relations\", \"reports\", or \"actions\"."),
				mcp.Enum("fields", "relations", "reports", "actions"),
			),
			mcp.Property("field_filter",
				mcp.Description("Optional substring filter applied to field keys/names."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleDescribeEvidence(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[describeEvidenceArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	if a.Section == "" {
		a.Section = "fields"
	}
	result, err := s.svc.DescribeEvidence(ctx, a.Company, a.Evidence, a.Section, a.FieldFilter)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_query_records ───────────────────────────────────────────────────────

func (s *Server) toolQueryRecords() *mcp.ServerTool {
	t := mcp.NewServerTool[queryRecordsArgs, any](
		"flexi_query_records",
		"Queries records from a FlexiBee evidence. Supports Flexi filter "+
			"expressions, field projection, sorting, and paging. Returns records "+
			"together with paging context and the resolved company.",
		s.handleQueryRecords,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Evidence key, e.g. \"vydane-faktury\"."),
				mcp.Required(true),
			),
			mcp.Property("filter",
				mcp.Description("Flexi filter expression, e.g. "+
					"\"stav='stavOdeslana' AND datSplat<'today'\"."),
			),
			mcp.Property("fields",
				mcp.Description("Comma-separated list of field keys to return. "+
					"Omit for all fields."),
			),
			mcp.Property("sort",
				mcp.Description("Sort expression, e.g. \"datSplat@A\" for ascending."),
			),
			mcp.Property("limit",
				mcp.Description("Maximum records to return (1–200, default 50)."),
			),
			mcp.Property("start",
				mcp.Description("Zero-based offset for pagination."),
			),
			mcp.Property("add_count",
				mcp.Description("When true, include the total record count in the response."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleQueryRecords(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[queryRecordsArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	if a.Limit <= 0 {
		a.Limit = defaultLimit
	}
	if a.Limit > 200 {
		a.Limit = 200
	}
	// a.Sort is a raw sort expression string (e.g. "datSplat@A").
	// Typed SortField parsing is deferred to WP6 (query builder).
	result, err := s.svc.QueryRecords(ctx, &service.QueryRequest{
		Company:  a.Company,
		Evidence: a.Evidence,
		Filter:   flexi.FilterExpr(a.Filter),
		Fields:   a.Fields,
		Limit:    a.Limit,
		Start:    a.Start,
		AddCount: a.AddCount,
	})
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_get_record ──────────────────────────────────────────────────────────

func (s *Server) toolGetRecord() *mcp.ServerTool {
	t := mcp.NewServerTool[getRecordArgs, any](
		"flexi_get_record",
		"Returns a single FlexiBee record by its identifier (numeric id, external "+
			"code, or other supported selector). Returns the resolved company in the "+
			"response.",
		s.handleGetRecord,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Evidence key."),
				mcp.Required(true),
			),
			mcp.Property("identifier",
				mcp.Description("Record identifier: numeric id, \"code:ABC\", "+
					"\"ext:EXT123\", etc."),
				mcp.Required(true),
			),
			mcp.Property("fields",
				mcp.Description("Field keys to project. Omit for all fields."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetRecord(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getRecordArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.GetRecord(ctx, a.Company, a.Evidence, a.Identifier, a.Fields)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_get_related_records ─────────────────────────────────────────────────

func (s *Server) toolGetRelatedRecords() *mcp.ServerTool {
	t := mcp.NewServerTool[getRelatedRecordsArgs, any](
		"flexi_get_related_records",
		"Returns records linked to a given record via a named evidence relation "+
			"(e.g. items of an invoice, payments of a document).",
		s.handleGetRelatedRecords,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Parent evidence key."),
				mcp.Required(true),
			),
			mcp.Property("identifier",
				mcp.Description("Parent record identifier."),
				mcp.Required(true),
			),
			mcp.Property("relation",
				mcp.Description("Relation name as returned by flexi_describe_evidence "+
					"with section=\"relations\"."),
				mcp.Required(true),
			),
			mcp.Property("limit",
				mcp.Description("Maximum records to return (default 50)."),
			),
			mcp.Property("start",
				mcp.Description("Zero-based offset for pagination."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetRelatedRecords(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getRelatedRecordsArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	if a.Limit <= 0 {
		a.Limit = defaultLimit
	}
	result, err := s.svc.GetRelatedRecords(ctx, a.Company, a.Evidence, a.Identifier, a.Relation, a.Limit, a.Start)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_sum_records ─────────────────────────────────────────────────────────

func (s *Server) toolSumRecords() *mcp.ServerTool {
	t := mcp.NewServerTool[sumRecordsArgs, any](
		"flexi_sum_records",
		"Returns document totals / aggregate sums for a FlexiBee evidence. "+
			"Useful for computing outstanding receivables, inventory totals, etc.",
		s.handleSumRecords,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Evidence key."),
				mcp.Required(true),
			),
			mcp.Property("filter",
				mcp.Description("Flexi filter expression to restrict which records "+
					"are included in the sum."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleSumRecords(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[sumRecordsArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.SumRecords(ctx, a.Company, a.Evidence, flexi.FilterExpr(a.Filter))
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_get_guide ───────────────────────────────────────────────────────────

func (s *Server) toolGetGuide() *mcp.ServerTool {
	t := mcp.NewServerTool[getGuideArgs, any](
		"flexi_get_guide",
		"Returns workflow guidance and API tips for a specific topic. Call this "+
			"first when unsure how to proceed with a FlexiBee task.",
		s.handleGetGuide,
		mcp.Input(
			mcp.Property("topic",
				mcp.Description("Guide topic to retrieve."),
				mcp.Enum("overview", "invoices", "contacts", "filtering", "paging",
					"connection", "write-operations", "actions"),
				mcp.Required(true),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetGuide(
	_ context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getGuideArgs],
) (*mcp.CallToolResult, error) {
	text, ok := guideTexts[params.Arguments.Topic]
	if !ok {
		return toolError(fmt.Errorf("unknown guide topic %q; valid topics: %v",
			params.Arguments.Topic, guideTopics)), nil
	}
	return toolText(text), nil
}

// ─── Prompt registration ──────────────────────────────────────────────────────

func (s *Server) registerPrompts() {
	s.server.AddPrompts(
		s.promptReviewReceivables(),
		s.promptPrepareInvoice(),
		s.promptInspectInventory(),
	)
}

func (s *Server) promptReviewReceivables() *mcp.ServerPrompt {
	return &mcp.ServerPrompt{
		Prompt: &mcp.Prompt{
			Name:        "review_receivables",
			Description: "Reviews unpaid and overdue outbound invoices for a given company.",
			Arguments: []*mcp.PromptArgument{
				{Name: "company", Description: "FlexiBee company db identifier.", Required: true},
				{Name: "days_overdue", Description: "Only include invoices overdue by at least this many days (default 0).", Required: false},
			},
		},
		Handler: func(_ context.Context, _ *mcp.ServerSession, p *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
			company := p.Arguments["company"]
			daysOverdue := p.Arguments["days_overdue"]
			if daysOverdue == "" {
				daysOverdue = "0"
			}
			userMsg := fmt.Sprintf(
				"Review unpaid and overdue outbound invoices for company %q.\n"+
					"Include only invoices overdue by at least %s days.\n"+
					"Steps:\n"+
					"1. Call flexi_query_records with evidence=\"vydane-faktury\" and a filter for unpaid status.\n"+
					"2. Identify overdue invoices (datSplat < today).\n"+
					"3. Summarise total outstanding amount and list the top overdue items.\n"+
					"4. Suggest next actions (reminders, write-offs, etc.).",
				company, daysOverdue,
			)
			return &mcp.GetPromptResult{
				Description: "Workflow for reviewing unpaid/overdue receivables.",
				Messages: []*mcp.PromptMessage{
					{Role: "user", Content: &mcp.TextContent{Text: userMsg}},
				},
			}, nil
		},
	}
}

func (s *Server) promptPrepareInvoice() *mcp.ServerPrompt {
	return &mcp.ServerPrompt{
		Prompt: &mcp.Prompt{
			Name:        "prepare_invoice",
			Description: "Guides the creation of a new outbound invoice in FlexiBee.",
			Arguments: []*mcp.PromptArgument{
				{Name: "company", Description: "FlexiBee company db identifier.", Required: true},
				{Name: "customer", Description: "Customer name or code.", Required: false},
			},
		},
		Handler: func(_ context.Context, _ *mcp.ServerSession, p *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
			company := p.Arguments["company"]
			customer := p.Arguments["customer"]
			hint := ""
			if customer != "" {
				hint = fmt.Sprintf(" for customer %q", customer)
			}
			userMsg := fmt.Sprintf(
				"Prepare a new outbound invoice%s in company %q.\n"+
					"Steps:\n"+
					"1. Look up the contact in evidence=\"adresar\" using flexi_query_records.\n"+
					"2. Retrieve the evidence schema with flexi_describe_evidence (evidence=\"vydane-faktury\") "+
					"to understand required fields.\n"+
					"3. Draft the invoice JSON with mandatory fields: firma, datVyst, datSplat, polozkyFaktury.\n"+
					"4. Confirm the data with the user before any write operation (Phase 2).",
				hint, company,
			)
			return &mcp.GetPromptResult{
				Description: "Workflow for drafting a new outbound invoice.",
				Messages: []*mcp.PromptMessage{
					{Role: "user", Content: &mcp.TextContent{Text: userMsg}},
				},
			}, nil
		},
	}
}

func (s *Server) promptInspectInventory() *mcp.ServerPrompt {
	return &mcp.ServerPrompt{
		Prompt: &mcp.Prompt{
			Name:        "inspect_inventory",
			Description: "Inspects current stock levels and highlights items below reorder threshold.",
			Arguments: []*mcp.PromptArgument{
				{Name: "company", Description: "FlexiBee company db identifier.", Required: true},
				{Name: "threshold", Description: "Reorder threshold – flag items whose stock is below this value (default 0).", Required: false},
			},
		},
		Handler: func(_ context.Context, _ *mcp.ServerSession, p *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
			company := p.Arguments["company"]
			threshold := p.Arguments["threshold"]
			if threshold == "" {
				threshold = "0"
			}
			userMsg := fmt.Sprintf(
				"Inspect stock levels in company %q and flag items below threshold %s.\n"+
					"Steps:\n"+
					"1. Query evidence=\"skladove-karty\" via flexi_query_records.\n"+
					"2. Filter or sort by current stock quantity.\n"+
					"3. List items below the threshold with their current quantities.\n"+
					"4. Suggest reorder quantities based on historical usage if data is available.",
				company, threshold,
			)
			return &mcp.GetPromptResult{
				Description: "Workflow for inspecting inventory and flagging low-stock items.",
				Messages: []*mcp.PromptMessage{
					{Role: "user", Content: &mcp.TextContent{Text: userMsg}},
				},
			}, nil
		},
	}
}

// ─── Resource registration ────────────────────────────────────────────────────

func (s *Server) registerResources() {
	for _, topic := range guideTopics {
		topic := topic // capture loop variable
		uri := "flexi://guides/" + topic
		text := guideTexts[topic]
		s.server.AddResources(&mcp.ServerResource{
			Resource: &mcp.Resource{
				URI:         uri,
				Name:        "FlexiBee guide: " + topic,
				Description: "Workflow and API guidance for topic: " + topic,
				MIMEType:    "text/plain",
			},
			Handler: func(_ context.Context, _ *mcp.ServerSession, _ *mcp.ReadResourceParams) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{
					Contents: []*mcp.ResourceContents{
						{URI: uri, MIMEType: "text/plain", Text: text},
					},
				}, nil
			},
		})
	}
}

// ─── Guide content ────────────────────────────────────────────────────────────

var guideTopics = []string{
	"overview", "invoices", "contacts", "filtering", "paging", "connection",
	"write-operations", "actions",
}

var guideTexts = map[string]string{
	"overview": `# FlexiBee MCP – Overview

FlexiBee (now ABRA Flexi) is a Czech ERP system exposing a REST/JSON API.
This MCP server provides read and write access to FlexiBee data.

## Tool summary

| Tool                        | R/W | Purpose                                     |
|-----------------------------|-----|---------------------------------------------|
| flexi_get_connection_info   | R   | Connectivity, active transport tier         |
| flexi_list_companies        | R   | Enumerate available companies               |
| flexi_list_evidences        | R   | Browse the evidence (table) catalog         |
| flexi_describe_evidence     | R   | Fields/relations/reports/actions schema     |
| flexi_query_records         | R   | Filter, sort and page records               |
| flexi_get_record            | R   | Single record by identifier                 |
| flexi_get_related_records   | R   | Traverse evidence relations                 |
| flexi_sum_records           | R   | Aggregate totals for document evidences     |
| flexi_list_actions          | R   | List available business actions             |
| flexi_get_report            | R   | Download report as base64 (PDF/XLSX/…)      |
| flexi_list_attachments      | R   | List attachments on a record                |
| flexi_get_attachment        | R   | Download attachment as base64               |
| flexi_get_changes           | R   | Changes API: incremental sync feed          |
| flexi_get_company_stats     | R   | Company statistics (record counts, size)    |
| flexi_list_saved_queries    | R   | List named saved queries                    |
| flexi_run_saved_query       | R   | Execute a named saved query                 |
| flexi_upsert_record         | W   | Create or update a record (dry_run support) |
| flexi_delete_record         | W   | Delete a record                             |
| flexi_execute_action        | W   | Execute a business action on a record       |
| flexi_get_guide             | R   | Workflow guidance text                      |

## Typical read workflow

1. Call flexi_get_connection_info to confirm connectivity and active transport.
2. Call flexi_list_companies to discover available companies.
3. Call flexi_list_evidences to find the relevant data table.
4. Call flexi_describe_evidence to learn required fields.
5. Call flexi_query_records / flexi_get_record to retrieve data.
6. Call flexi_sum_records for financial aggregates.

## Typical write workflow

1. Call flexi_describe_evidence to identify required fields.
2. Call flexi_upsert_record with dry_run=true to validate.
3. Call flexi_upsert_record to persist when the dry run succeeds.
4. Call flexi_list_actions + flexi_execute_action for workflows.

## Transport tiers (FLEXIBEE_AUTH_MODE=auto by default)

1. API (Basic Auth) – direct REST API; requires rest-api-rw licence
2. Session (v2 HTML auth) – mimics web browser; no licence required
3. Java subprocess – native Java client; fallback using Java's HTTP stack

The active transport is shown in flexi_get_connection_info as "active_transport".

## Authentication

Credentials are loaded from environment variables (see topic "connection").
They are never echoed in tool responses.
`,

	"invoices": `# FlexiBee MCP – Invoices guide

## Key evidence identifiers

| Evidence key        | Description                      |
|---------------------|----------------------------------|
| vydane-faktury      | Outbound (sales) invoices        |
| prijate-faktury     | Inbound (purchase) invoices      |
| vydane-zalohove-faktury | Outbound advance invoices    |

## Useful fields (vydane-faktury)

- id, kod, popis          – internal id, code, description
- firma / firmaStr        – linked contact / resolved name
- datVyst                 – issue date (YYYY-MM-DD)
- datSplat                – due date
- sumCelkem               – total incl. VAT
- stavUhrK / stavUhrKStr  – payment status key / label
- stav                    – document status (stavOdeslana = sent, etc.)

## Common queries

Unpaid sent invoices:
  filter = "stavUhrK='stavUhrNeuhrazeno' AND stav='stavOdeslana'"

Overdue invoices:
  filter = "stavUhrK='stavUhrNeuhrazeno' AND datSplat<'today'"

Invoice for a specific contact (ICO = 12345678):
  filter = "ic='12345678'"

## Related records

Call flexi_get_related_records with relation="polozkyFaktury" to get
line items; use relation="uhradyFaktury" for linked payments.
`,

	"contacts": `# FlexiBee MCP – Contacts guide

## Key evidence

| Evidence key | Description                      |
|--------------|----------------------------------|
| adresar      | Address book / contacts          |

## Useful fields

- id, kod, nazev           – id, code, display name
- ic, dic                  – company registration number, VAT number
- email, tel               – contact details
- ulice, mesto, psc, stat  – address
- typVztahuK               – relationship type (customer, supplier, …)

## Searching contacts

By name substring (Flexi uses LIKE with %):
  filter = "nazev LIKE '%Novák%'"

By ICO:
  filter = "ic='27074358'"

By relationship type (customer only):
  filter = "typVztahuK='typVztahuOdberatel'"

## Tips

Use flexi_describe_evidence with evidence="adresar" to see all
available fields and their types before querying.
`,

	"filtering": `# FlexiBee MCP – Filtering guide

FlexiBee uses its own SQL-like filter expression syntax.

## Operators

=, <>, <, <=, >, >=     Standard comparisons
LIKE 'pattern%'          String matching (% = any chars, _ = one char)
IN (v1, v2, ...)         Set membership
IS NULL / IS NOT NULL    Null checks
AND, OR, NOT             Logical operators

## Date literals

today, yesterday, lastMonth, thisMonth, thisYear
Relative: today-7   (7 days ago)
Absolute: '2024-01-31'

## Examples

Unpaid invoices due this month:
  stavUhrK='stavUhrNeuhrazeno' AND datSplat>=firstDayOfMonth AND datSplat<=lastDayOfMonth

Documents over 10 000 CZK:
  sumCelkem>10000

Contacts in Prague:
  mesto='Praha'

Multi-value:
  stav IN ('stavOdeslana','stavZauctovana')

## Notes

- String values must be single-quoted.
- Field names are camelCase (check schema with flexi_describe_evidence).
- Use addCount=true in flexi_query_records to get the total result count.
`,

	"paging": `# FlexiBee MCP – Paging guide

All list tools support cursor-based paging via the "start" and "limit"
parameters.

## Parameters

start  – Zero-based index of the first record to return (default 0).
limit  – Maximum number of records to return (default 50, max 200).

## Iteration pattern

page := 0
limit := 50
for {
    result = flexi_query_records(... start=page*limit, limit=limit, add_count=true)
    process(result.records)
    if len(result.records) < limit { break }
    page++
}

## Tips

- Set add_count=true on the first call to know the total count.
- Avoid fetching more than 200 records in one call; use paging instead.
- The "limit" default of 50 is chosen to stay well within context-window
  budgets for most LLM interactions.
`,

	"connection": `# FlexiBee MCP – Connection configuration

## Required environment variables

FLEXIBEE_URL       Base URL of the FlexiBee server, e.g.
                   https://demo.flexibee.eu:5434
FLEXIBEE_USERNAME  Username for authentication
FLEXIBEE_PASSWORD  Password (mutually exclusive with FLEXIBEE_PASSWORD_FILE)

## Optional environment variables

FLEXIBEE_PASSWORD_FILE  Path to a file containing the password (newline-trimmed)
FLEXIBEE_COMPANY        Default company db identifier used when tools omit "company"
FLEXIBEE_TIMEOUT_MS     HTTP timeout in milliseconds (default 30000, min 1000)
FLEXIBEE_PAGE_SIZE      Default page size (default 50, max 200)
FLEXIBEE_CA_FILE        Path to a custom CA certificate (PEM) for self-signed certs
FLEXIBEE_ALLOW_HTTP     Set to "true" to permit plain http:// on loopback addresses
FLEXIBEE_SKIP_TLS_VERIFY Set to "true" to skip TLS verification (dev only)

## Authentication modes (FLEXIBEE_AUTH_MODE)

auto     (default) API first → session on 402 → Java subprocess as last resort
api      HTTP Basic Auth REST – requires rest-api-rw licence (apiUsers > 0)
session  v2 Session Auth – mimics web browser, no API licence required
         Uses /v2/ endpoint prefix with CSRF token, User-Agent: WinStrom/DESKTOP
java     Native Java subprocess – uses Java's HTTP stack with WinStrom User-Agent
         Requires Java 11+ (JDK); automatically found at well-known paths
         or override with FLEXIBEE_JAVA_HOME

## Transport priority in "auto" mode

1. API (Basic Auth) – tried first; if HTTP 402 received, falls back
2. Session (v2 HTML auth) – bypasses licence check; proven working on basic2016
3. Java subprocess – native Java client mimic; fallback if Java JDK is available

## Security notes

- Credentials are never logged or included in MCP tool responses.
- Use FLEXIBEE_PASSWORD_FILE to avoid exposing passwords in process env.
- FLEXIBEE_ALLOW_HTTP is only permitted for loopback hosts (127.0.0.1, ::1, localhost).
- FLEXIBEE_SKIP_TLS_VERIFY disables certificate checks; only for development.
- For session mode, use a dedicated user account (e.g. "mcpbot") to avoid
  conflicting with active browser sessions (one session per user per client type).
`,

	"write-operations": `# FlexiBee MCP – Write operations guide

## Overview

Write tools mutate FlexiBee data. Always use dry_run=true first to validate.

## flexi_upsert_record

Creates a new record or updates an existing one.

### Create a new contact

{
  "company": "my-company",
  "evidence": "adresar",
  "record": {
    "nazev": "ACME s.r.o.",
    "ic": "12345678",
    "email": "info@acme.cz",
    "typVztahuK": "typVztahuOdberatel"
  }
}

### Update an existing record (by id)

{
  "company": "my-company",
  "evidence": "adresar",
  "record": {
    "id": "42",
    "email": "new@acme.cz"
  }
}

### Dry run (validate only, no write)

Set "dry_run": true to calculate totals and validate required fields
without persisting. Useful for invoice preview.

## flexi_delete_record

Deletes a record. Cannot be undone. The server rejects deletion if the
record is referenced by other records.

{
  "company": "my-company",
  "evidence": "adresar",
  "identifier": "42"
}

## Record identifiers

Flexi selectors accepted as "identifier":
  - Numeric id: "42"
  - External code: "ext:MYSYSTEM:REF123"
  - Business code: "code:CUS001"

## Winstrom envelope

You do not need to wrap your record. flexi_upsert_record wraps it
automatically in {"winstrom": {"<evidence>": {fields}}} before sending.

## Important notes

- Field names are camelCase (check flexi_describe_evidence first).
- Mandatory fields depend on evidence type; dry run reveals missing fields.
- Relation fields (e.g. "firma") accept id selectors: {"firma": {"id": "5"}}.
- Date fields use ISO 8601 format: "2024-12-31".
`,

	"actions": `# FlexiBee MCP – Business actions guide

## Overview

Actions trigger server-side workflows on existing records.
Use flexi_list_actions to discover available actions, then
flexi_execute_action to invoke one.

## Common actions (faktura-vydana / invoice)

| actionPath | Description |
|------------|-------------|
| lock       | Lock the document (prevents further editing) |
| unlock     | Unlock the document |
| storno     | Cancel / void the document |
| odeslat    | Mark as sent |
| zaplatit   | Mark as paid |

## Workflow example – send an invoice

1. Query the invoice: flexi_get_record
2. List available actions: flexi_list_actions (evidence="faktura-vydana")
3. Execute: flexi_execute_action
   {
     "company": "my-company",
     "evidence": "faktura-vydana",
     "identifier": "123",
     "action": "odeslat"
   }

## Notes

- Not all actions are available on all records (depends on document state).
- Actions are evidence-specific; run flexi_list_actions for each evidence type.
- Some actions have side effects (e.g. "zaplatit" creates a payment record).
`,
}
