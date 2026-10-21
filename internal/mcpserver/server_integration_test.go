// Package mcpserver – server_integration_test.go runs the flexibee-mcp binary
// as a stdio child, sends MCP JSON-RPC messages over stdin (content-length
// framing), reads responses from stdout, and verifies:
//
//   - The MCP initialize / initialized handshake succeeds.
//   - Each registered tool can be called and returns a valid result.
//   - No non-JSON-RPC output leaks to stdout.
//
// A TLS httptest server mimics FlexiBee so no real instance is needed.
package mcpserver_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── Package setup ────────────────────────────────────────────────────────────

var integrationBin string

func TestMain(m *testing.M) {
	bin, err := buildBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcpserver_test: build failed: %v\n", err)
		os.Exit(1)
	}
	integrationBin = bin
	os.Exit(m.Run())
}

func buildBinary() (string, error) {
	dir, err := os.MkdirTemp("", "flexibee-mcp-int-*")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "flexibee-mcp")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/flexibee-mcp/")
	cmd.Dir = moduleRoot()
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v\n%s", err, b)
	}
	return out, nil
}

func moduleRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dir
}

// ─── Mock FlexiBee server ─────────────────────────────────────────────────────

func startMockFlexi(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	jsonOK := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	}

	mux.HandleFunc("/v2/login-logout/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"authSessionId": "int-sid", "csrfToken": "int-csrf", "success": true,
		})
	})
	mux.HandleFunc("/v2/login-logout/session-keep-alive.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"success":true}`)
	})
	mux.HandleFunc("/v2/c.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"companies":{"company":[{"dbNazev":"test-co","nazev":"Test"}]}}`)
	})
	mux.HandleFunc("/v2/c/test-co/evidence-list.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"evidences":{"evidence":[{"evidenceName":"Adresář","evidencePath":"adresar"}]}}`)
	})
	mux.HandleFunc("/v2/c/test-co/adresar/properties.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"properties":{"property":[{"name":"id","title":"ID"},{"name":"nazev","title":"Název"}]}}`)
	})
	mux.HandleFunc("/v2/c/test-co/adresar/relations.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"relations":{"relation":[{"relationPath":"kontakty","name":"Kontakty"}]}}`)
	})
	mux.HandleFunc("/v2/c/test-co/adresar/reports.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"reports":{"report":[]}}`)
	})
	mux.HandleFunc("/v2/c/test-co/adresar/actions.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"actions":{"action":[{"actionPath":"email","actionName":"Send"}]}}`)
	})
	mux.HandleFunc("/v2/c/test-co/adresar/query.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"winstrom":{"adresar":[{"id":"1","nazev":"ACME"}]}}`)
	})
	// single record and related by path prefix
	mux.HandleFunc("/v2/c/test-co/adresar/", func(w http.ResponseWriter, r *http.Request) {
		// Route sub-paths: prilohy, content, etc. fall through to /v2/ catch-all.
		// Single record or related: return mock data.
		path := r.URL.Path
		if strings.Contains(path, "/prilohy.json") {
			jsonOK(w, `{"winstrom":{"priloha":[{"id":"10","nazev":"doc.pdf","contentType":"application/pdf"}]}}`)
			return
		}
		if strings.Contains(path, "/a/") {
			jsonOK(w, `{"winstrom":{}}`)
			return
		}
		jsonOK(w, `{"winstrom":{"adresar":[{"id":"1","nazev":"ACME"}]}}`)
	})
	// report download
	mux.HandleFunc("/v2/c/test-co/adresar/etiketa.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 mock"))
	})
	// attachment content
	mux.HandleFunc("/v2/c/test-co/priloha/10/content", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("mock pdf content"))
	})
	// changes
	mux.HandleFunc("/v2/c/test-co/changes.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"winstrom":{"@lastEventId":"42","changes":[{"id":"1","evidence":"adresar","method":"UPDATE"}]}}`)
	})
	// company statistics
	mux.HandleFunc("/v2/c/test-co/statistics.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"winstrom":{"statistics":[{"evidence":"adresar","count":"100"}]}}`)
	})
	// saved queries list
	mux.HandleFunc("/v2/c/test-co/uzivatelsky-dotaz.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"winstrom":{"uzivatelsky-dotaz":[{"id":"99","kod":"overdue","nazev":"Overdue invoices"}]}}`)
	})
	// saved query execution
	mux.HandleFunc("/v2/c/test-co/uzivatelsky-dotaz/99/call.json", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"winstrom":{"faktura-vydana":[{"id":"1","kod":"F001","sumCelkem":"1000"}],"@rowCount":"1"}}`)
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, `{"winstrom":{}}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"winstrom":{"@globalError":"PaymentRequired"}}`, http.StatusPaymentRequired)
	})

	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ─── MCP process wrapper ──────────────────────────────────────────────────────

type mcpProc struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	scan  *bufio.Scanner
	msgID int
}

func startMCPProc(t *testing.T, flexiURL string) *mcpProc {
	t.Helper()
	cmd := exec.Command(integrationBin)
	cmd.Env = append(os.Environ(),
		"FLEXIBEE_URL="+flexiURL,
		"FLEXIBEE_USERNAME=u",
		"FLEXIBEE_PASSWORD=p",
		"FLEXIBEE_COMPANY=test-co",
		"FLEXIBEE_AUTH_MODE=session",
		"FLEXIBEE_SKIP_TLS_VERIFY=true",
	)
	cmd.Stderr = os.Stderr

	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return &mcpProc{cmd: cmd, in: in, scan: bufio.NewScanner(out)}
}

// send emits a newline-delimited JSON-RPC request (go-sdk StdioTransport uses NDJSON).
func (p *mcpProc) send(t *testing.T, method string, params interface{}) {
	t.Helper()
	p.msgID++
	var msg map[string]interface{}
	if strings.HasPrefix(method, "notifications/") {
		msg = map[string]interface{}{"jsonrpc": "2.0", "method": method}
	} else {
		msg = map[string]interface{}{"jsonrpc": "2.0", "id": p.msgID, "method": method}
	}
	if params != nil {
		msg["params"] = params
	}
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	if _, err := p.in.Write(b); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

// recv reads the next newline-delimited JSON-RPC response.
func (p *mcpProc) recv(t *testing.T) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !p.scan.Scan() {
			if p.scan.Err() != nil {
				t.Fatalf("scan: %v", p.scan.Err())
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		line := strings.TrimSpace(p.scan.Text())
		if line == "" {
			continue
		}
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		return resp
	}
	t.Fatal("timed out waiting for MCP response")
	return nil
}

// handshake performs initialize / initialized and returns the initialize result.
func (p *mcpProc) handshake(t *testing.T) map[string]interface{} {
	t.Helper()
	p.send(t, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "test", "version": "0.1"},
	})
	resp := p.recv(t)
	p.send(t, "notifications/initialized", nil)
	return resp
}

// callTool calls a tool and returns the result object.
func (p *mcpProc) callTool(t *testing.T, name string, args interface{}) map[string]interface{} {
	t.Helper()
	p.send(t, "tools/call", map[string]interface{}{"name": name, "arguments": args})
	resp := p.recv(t)
	if errObj, ok := resp["error"]; ok {
		t.Fatalf("tool %q returned JSON-RPC error: %v", name, errObj)
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool %q result not an object: %v", name, resp)
	}
	return result
}

// contentText extracts text from a CallToolResult content array.
func contentText(result map[string]interface{}) string {
	items, _ := result["content"].([]interface{})
	var sb strings.Builder
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if m != nil {
			if text, ok := m["text"].(string); ok {
				sb.WriteString(text)
			}
		}
	}
	return sb.String()
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestMCPHandshake(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	resp := p.handshake(t)
	if _, ok := resp["result"]; !ok {
		t.Fatalf("initialize: no result field: %v", resp)
	}
	if resp["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want \"2.0\"", resp["jsonrpc"])
	}
}

func TestMCPServerInfo(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	resp := p.handshake(t)
	result := resp["result"].(map[string]interface{})
	if _, ok := result["serverInfo"]; !ok {
		t.Error("initialize result missing serverInfo")
	}
}

func TestMCPStdoutIsJSONRPCOnly(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	resp := p.handshake(t)
	if resp["jsonrpc"] != "2.0" {
		t.Errorf("stdout contained non-JSON-RPC content: %v", resp)
	}
}

func TestToolGetConnectionInfo(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_get_connection_info", map[string]interface{}{})
	if result["isError"] == true {
		t.Errorf("unexpected isError: %v", contentText(result))
	}
}

func TestToolListCompanies(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_list_companies", map[string]interface{}{})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
	if text := contentText(result); !strings.Contains(text, "test-co") {
		t.Errorf("list_companies = %q; want to contain \"test-co\"", text)
	}
}

func TestToolListEvidences(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_list_evidences", map[string]interface{}{"company": "test-co"})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
	if text := contentText(result); !strings.Contains(text, "adresar") {
		t.Errorf("list_evidences = %q; want \"adresar\"", text)
	}
}

func TestToolDescribeEvidenceFields(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_describe_evidence", map[string]interface{}{
		"company": "test-co", "evidence": "adresar", "section": "fields",
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
	if text := contentText(result); !strings.Contains(text, "nazev") {
		t.Errorf("describe_evidence/fields = %q; want \"nazev\"", text)
	}
}

func TestToolDescribeEvidenceRelations(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_describe_evidence", map[string]interface{}{
		"company": "test-co", "evidence": "adresar", "section": "relations",
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
}

func TestToolDescribeEvidenceReports(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_describe_evidence", map[string]interface{}{
		"company": "test-co", "evidence": "adresar", "section": "reports",
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
}

func TestToolDescribeEvidenceActions(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_describe_evidence", map[string]interface{}{
		"company": "test-co", "evidence": "adresar", "section": "actions",
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
}

func TestToolQueryRecords(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_query_records", map[string]interface{}{
		"company": "test-co", "evidence": "adresar", "limit": 10,
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
	if text := contentText(result); !strings.Contains(text, "ACME") {
		t.Errorf("query_records = %q; want \"ACME\"", text)
	}
}

func TestToolGetRecord(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_get_record", map[string]interface{}{
		"company": "test-co", "evidence": "adresar", "identifier": "1",
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
}

func TestToolGetRelatedRecords(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_get_related_records", map[string]interface{}{
		"company": "test-co", "evidence": "adresar",
		"identifier": "1", "relation": "kontakty",
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
}

func TestToolListActions(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_list_actions", map[string]interface{}{
		"company": "test-co", "evidence": "adresar",
	})
	if result["isError"] == true {
		t.Errorf("error: %v", contentText(result))
	}
}

func TestToolGetGuideAllTopics(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	topics := []string{
		"overview", "invoices", "contacts", "filtering",
		"paging", "connection", "write-operations", "actions",
	}
	for _, topic := range topics {
		result := p.callTool(t, "flexi_get_guide", map[string]interface{}{"topic": topic})
		if result["isError"] == true {
			t.Errorf("flexi_get_guide(%q) error: %v", topic, contentText(result))
		}
		if text := contentText(result); text == "" {
			t.Errorf("flexi_get_guide(%q): empty content", topic)
		}
	}
}

func TestToolGetReport(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_get_report", map[string]interface{}{
		"company": "test-co", "evidence": "adresar",
		"report_path": "etiketa", "format": "pdf",
	})
	if result["isError"] == true {
		t.Errorf("flexi_get_report error: %v", contentText(result))
	}
	text := contentText(result)
	if !strings.Contains(text, "data_base64") {
		t.Errorf("flexi_get_report = %q; want data_base64 field", text)
	}
}

func TestToolListAttachments(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_list_attachments", map[string]interface{}{
		"company": "test-co", "evidence": "adresar", "identifier": "1",
	})
	if result["isError"] == true {
		t.Errorf("flexi_list_attachments error: %v", contentText(result))
	}
}

func TestToolGetAttachment(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_get_attachment", map[string]interface{}{
		"company": "test-co", "attachment_id": "10",
	})
	if result["isError"] == true {
		t.Errorf("flexi_get_attachment error: %v", contentText(result))
	}
	text := contentText(result)
	if !strings.Contains(text, "data_base64") {
		t.Errorf("flexi_get_attachment = %q; want data_base64 field", text)
	}
}

func TestToolGetChanges(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_get_changes", map[string]interface{}{
		"company": "test-co", "last_event_id": 0,
	})
	if result["isError"] == true {
		t.Errorf("flexi_get_changes error: %v", contentText(result))
	}
}

func TestToolGetCompanyStats(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_get_company_stats", map[string]interface{}{
		"company": "test-co",
	})
	if result["isError"] == true {
		t.Errorf("flexi_get_company_stats error: %v", contentText(result))
	}
}

func TestToolListSavedQueries(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_list_saved_queries", map[string]interface{}{
		"company": "test-co",
	})
	if result["isError"] == true {
		t.Errorf("flexi_list_saved_queries error: %v", contentText(result))
	}
	if text := contentText(result); !strings.Contains(text, "overdue") {
		t.Errorf("list_saved_queries = %q; want to contain \"overdue\"", text)
	}
}

func TestToolRunSavedQuery(t *testing.T) {
	srv := startMockFlexi(t)
	p := startMCPProc(t, srv.URL)
	p.handshake(t)
	result := p.callTool(t, "flexi_run_saved_query", map[string]interface{}{
		"company": "test-co", "query_id": "99", "limit": 10,
	})
	if result["isError"] == true {
		t.Errorf("flexi_run_saved_query error: %v", contentText(result))
	}
	if text := contentText(result); !strings.Contains(text, "F001") {
		t.Errorf("run_saved_query = %q; want to contain \"F001\"", text)
	}
}
