// Package mcpserver – tools_write.go registers write-capable MCP tools.
//
// These tools expose mutating FlexiBee operations (upsert, delete, action).
// Each tool is annotated with ReadOnlyHint: false so that MCP clients can
// surface appropriate confirmation prompts before destructive actions.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opentreecz/flexibee-mcp/internal/flexi"
	"github.com/opentreecz/flexibee-mcp/internal/service"
)

// ── Input structs ─────────────────────────────────────────────────────────────

type upsertRecordArgs struct {
	Company  string                 `json:"company"`
	Evidence string                 `json:"evidence"`
	Record   map[string]interface{} `json:"record"`
	DryRun   bool                   `json:"dry_run,omitempty"`
}

type deleteRecordArgs struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	Identifier string `json:"identifier"`
}

type executeActionArgs struct {
	Company    string `json:"company"`
	Evidence   string `json:"evidence"`
	Identifier string `json:"identifier"`
	Action     string `json:"action"`
}

type listActionsArgs struct {
	Company  string `json:"company"`
	Evidence string `json:"evidence"`
}

// ── Registration ──────────────────────────────────────────────────────────────

func (s *Server) registerWriteTools() {
	tools := []*mcp.ServerTool{
		s.toolUpsertRecord(),
		s.toolDeleteRecord(),
		s.toolExecuteAction(),
		s.toolListActions(),
	}
	s.server.AddTools(tools...)
}

// ── flexi_upsert_record ───────────────────────────────────────────────────────

func (s *Server) toolUpsertRecord() *mcp.ServerTool {
	return mcp.NewServerTool[upsertRecordArgs, any](
		"flexi_upsert_record",
		"Creates or updates a record in a FlexiBee evidence. "+
			"Supply all required fields for a new record, or include the record "+
			"identifier (\"id\", \"code:ABC\", etc.) to update an existing one. "+
			"Use dry_run=true to validate without persisting. "+
			"Returns the server-side record as created/updated.",
		s.handleUpsertRecord,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Evidence key, e.g. \"adresar\" or \"faktura-vydana\"."),
				mcp.Required(true),
			),
			mcp.Property("record",
				mcp.Description("Field map for the record. Keys are FlexiBee field names "+
					"(camelCase). Use flexi_describe_evidence to discover required fields."),
				mcp.Required(true),
			),
			mcp.Property("dry_run",
				mcp.Description("When true, validate and calculate without persisting (default false)."),
			),
		),
	)
}

func (s *Server) handleUpsertRecord(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[upsertRecordArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.UpsertRecord(ctx, &service.UpsertRequest{
		Company:  a.Company,
		Evidence: a.Evidence,
		Record:   a.Record,
		DryRun:   a.DryRun,
	})
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_delete_record ───────────────────────────────────────────────────────

func (s *Server) toolDeleteRecord() *mcp.ServerTool {
	return mcp.NewServerTool[deleteRecordArgs, any](
		"flexi_delete_record",
		"Deletes a FlexiBee record by its identifier. "+
			"This is a destructive operation and cannot be undone. "+
			"If the record is referenced by other records, the server may reject "+
			"the delete with an application error.",
		s.handleDeleteRecord,
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
				mcp.Description("Record identifier: numeric id, \"code:ABC\", \"ext:EXT\", etc."),
				mcp.Required(true),
			),
		),
	)
}

func (s *Server) handleDeleteRecord(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[deleteRecordArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.DeleteRecord(ctx, a.Company, a.Evidence, a.Identifier)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_execute_action ──────────────────────────────────────────────────────

func (s *Server) toolExecuteAction() *mcp.ServerTool {
	return mcp.NewServerTool[executeActionArgs, any](
		"flexi_execute_action",
		"Executes a named business action on a FlexiBee record. "+
			"Actions trigger workflows such as locking, cancelling, or sending "+
			"documents. Use flexi_list_actions to discover available actions for "+
			"an evidence.",
		s.handleExecuteAction,
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
				mcp.Description("Record identifier: numeric id, \"code:ABC\", etc."),
				mcp.Required(true),
			),
			mcp.Property("action",
				mcp.Description("Action path as returned by flexi_list_actions, "+
					"e.g. \"lock\", \"cancel\", \"send\"."),
				mcp.Required(true),
			),
		),
	)
}

func (s *Server) handleExecuteAction(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[executeActionArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.ExecuteAction(ctx, a.Company, a.Evidence, a.Identifier, a.Action)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── flexi_list_actions ────────────────────────────────────────────────────────

func (s *Server) toolListActions() *mcp.ServerTool {
	t := mcp.NewServerTool[listActionsArgs, any](
		"flexi_list_actions",
		"Lists the available business actions for a FlexiBee evidence. "+
			"Use the returned actionPath values with flexi_execute_action.",
		s.handleListActions,
		mcp.Input(
			mcp.Property("company",
				mcp.Description("FlexiBee company db identifier."),
				mcp.Required(true),
			),
			mcp.Property("evidence",
				mcp.Description("Evidence key, e.g. \"faktura-vydana\"."),
				mcp.Required(true),
			),
		),
	)
	t.Tool.Annotations = readOnly()
	return t
}

func (s *Server) handleListActions(
	ctx context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[listActionsArgs],
) (*mcp.CallToolResult, error) {
	a := params.Arguments
	result, err := s.svc.ListActions(ctx, a.Company, a.Evidence)
	if err != nil {
		return toolError(err), nil
	}
	return toolJSON(result)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// toolJSONRaw returns a successful CallToolResult containing a raw JSON string
// that was already marshalled.  Used when the caller has pre-built JSON.
func toolJSONRaw(raw json.RawMessage) *mcp.CallToolResult {
	return toolText(string(raw))
}

// toolNotImplemented returns a polite error for tools not yet wired up.
func toolNotImplemented(name string) *mcp.CallToolResult {
	return toolError(fmt.Errorf("tool %q is not yet implemented", name))
}

// nonDestructive returns ToolAnnotations for tools that read but do not mutate.
func nonDestructive() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: false}
}

// ensure flexi and json packages are used
var _ = flexi.FilterExpr
var _ = json.Marshal
