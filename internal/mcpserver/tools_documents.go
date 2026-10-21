// Package mcpserver – tools_documents.go registers document-related MCP tools:
// report download, attachment listing/download, Changes API, company stats.
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opentreecz/flexibee-mcp/internal/flexi"
)

// ── Input structs ─────────────────────────────────────────────────────────────

type getReportArgs struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	ReportPath string `json:"report_path"`
	Format     string `json:"format,omitempty"`
	Filter     string `json:"filter,omitempty"`
}

type listAttachmentsArgs struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	Identifier string `json:"identifier"`
}

type getAttachmentArgs struct {
	Company      string `json:"company"`
	AttachmentID string `json:"attachment_id"`
}

type getChangesArgs struct {
	Company     string `json:"company"`
	LastEventID int64  `json:"last_event_id,omitempty"`
}

type getCompanyStatsArgs struct {
	Company string `json:"company"`
}

// ── Registration ──────────────────────────────────────────────────────────────

func (s *Server) registerDocumentTools() {
	tools := []*mcp.ServerTool{
		s.toolGetReport(),
		s.toolListAttachments(),
		s.toolGetAttachment(),
		s.toolGetChanges(),
		s.toolGetCompanyStats(),
		s.toolListSavedQueries(),
		s.toolRunSavedQuery(),
	}
	s.server.AddTools(tools...)
}

// ── flexi_get_report ──────────────────────────────────────────────────────────

func (s *Server) toolGetReport() *mcp.ServerTool {
	t := mcp.NewServerTool[getReportArgs, any](
		"flexi_get_report",
		"Downloads a FlexiBee report (print template) as base64-encoded bytes. "+
			"Use flexi_describe_evidence with section=\"reports\" to discover "+
			"available report paths. The returned data_base64 field contains the "+
			"full file (PDF, XLSX, CSV, etc.) encoded in standard base64.",
		s.handleGetReport,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Evidence key, e.g. \"faktura-vydana\"."),
				mcp.Required(true),
			),
			mcp.Property("report_path",
				mcp.Description("Report path as returned by flexi_describe_evidence "+
					"section=\"reports\", e.g. \"faktura\"."),
				mcp.Required(true),
			),
			mcp.Property("format",
				mcp.Description("Output format: pdf (default), xlsx, xls, csv, xml, isdoc."),
				mcp.Enum("pdf", "xlsx", "xls", "csv", "xml", "isdoc", "isdocx"),
			),
			mcp.Property("filter",
				mcp.Description("Flexi filter expression to restrict records included in the report."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetReport(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getReportArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	if a.Format == "" {
		a.Format = "pdf"
	}
	result, err := s.svc.GetReport(ctx, a.Company, a.Evidence, a.ReportPath, a.Format,
		flexi.FilterExpr(a.Filter))
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_list_attachments ────────────────────────────────────────────────────

func (s *Server) toolListAttachments() *mcp.ServerTool {
	t := mcp.NewServerTool[listAttachmentsArgs, any](
		"flexi_list_attachments",
		"Lists attachments on a FlexiBee record. Returns attachment metadata "+
			"including filename, size, and attachment id. Use flexi_get_attachment "+
			"with the returned id to download the actual file bytes.",
		s.handleListAttachments,
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
				mcp.Description("Parent record identifier."),
				mcp.Required(true),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleListAttachments(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[listAttachmentsArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.ListAttachments(ctx, a.Company, a.Evidence, a.Identifier)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_get_attachment ──────────────────────────────────────────────────────

func (s *Server) toolGetAttachment() *mcp.ServerTool {
	t := mcp.NewServerTool[getAttachmentArgs, any](
		"flexi_get_attachment",
		"Downloads a FlexiBee attachment by its id (from flexi_list_attachments) "+
			"and returns the file content as base64-encoded bytes. "+
			"The data_base64 field can be decoded to get the raw binary file.",
		s.handleGetAttachment,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("attachment_id",
				mcp.Description("Numeric attachment id from flexi_list_attachments."),
				mcp.Required(true),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetAttachment(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getAttachmentArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.GetAttachment(ctx, a.Company, a.AttachmentID)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_get_changes ─────────────────────────────────────────────────────────

func (s *Server) toolGetChanges() *mcp.ServerTool {
	t := mcp.NewServerTool[getChangesArgs, any](
		"flexi_get_changes",
		"Fetches records changed in FlexiBee since a given event ID using the "+
			"Changes API. Pass last_event_id=0 (or omit) for the full change log. "+
			"Store the returned last_event_id and pass it on the next call for "+
			"incremental sync. Returns evidence type, record id, timestamp, and "+
			"operation (INSERT/UPDATE/DELETE) for each change.",
		s.handleGetChanges,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("last_event_id",
				mcp.Description("Return changes with event ID > this value. "+
					"Use 0 or omit for the full log; pass the previous call's "+
					"last_event_id for incremental updates."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetChanges(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getChangesArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.GetChanges(ctx, a.Company, a.LastEventID)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_get_company_stats ───────────────────────────────────────────────────

func (s *Server) toolGetCompanyStats() *mcp.ServerTool {
	t := mcp.NewServerTool[getCompanyStatsArgs, any](
		"flexi_get_company_stats",
		"Returns statistics for the given FlexiBee company: record counts per "+
			"evidence, database size, last update timestamps, and similar "+
			"operational metrics.",
		s.handleGetCompanyStats,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleGetCompanyStats(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[getCompanyStatsArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.GetCompanyStats(ctx, a.Company)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_run_saved_query / flexi_list_saved_queries ──────────────────────────

type listSavedQueriesArgs struct {
	Company string `json:"company"`
}

type runSavedQueryArgs struct {
	Company string `json:"company"`
	QueryID string `json:"query_id"`
	Limit   int    `json:"limit,omitempty"`
	Start   int    `json:"start,omitempty"`
}

func (s *Server) toolListSavedQueries() *mcp.ServerTool {
	t := mcp.NewServerTool[listSavedQueriesArgs, any](
		"flexi_list_saved_queries",
		"Lists all named saved queries (user-defined queries) available in the "+
			"given company. Use the returned id or kod with flexi_run_saved_query "+
			"to execute one.",
		s.handleListSavedQueries,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleListSavedQueries(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[listSavedQueriesArgs],
) (*mcp.CallToolResult, error) {
	result, err := s.svc.ListSavedQueries(ctx, params.Arguments.Company)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

func (s *Server) toolRunSavedQuery() *mcp.ServerTool {
	t := mcp.NewServerTool[runSavedQueryArgs, any](
		"flexi_run_saved_query",
		"Executes a named saved query by its id or kod. Saved queries are "+
			"server-side named filter/sort configurations maintained by FlexiBee "+
			"users. Use flexi_list_saved_queries to discover available queries. "+
			"Returns paged records, total count, and paging metadata.",
		s.handleRunSavedQuery,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("query_id",
				mcp.Description("Saved query id or kod as returned by flexi_list_saved_queries."),
				mcp.Required(true),
			),
			mcp.Property("limit",
				mcp.Description("Maximum records to return (1–200, default 50)."),
			),
			mcp.Property("start",
				mcp.Description("Zero-based offset for pagination."),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleRunSavedQuery(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[runSavedQueryArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	if a.Limit <= 0 {
		a.Limit = defaultLimit
	}
	if a.Limit > 200 {
		a.Limit = 200
	}
	result, err := s.svc.RunSavedQuery(ctx, a.Company, a.QueryID, a.Limit, a.Start)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}
