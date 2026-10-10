// Package flexi – typed filter tree and URL/body query builder.
//
// # Filter tree
//
// Build a filter expression from composable FilterNode values:
//
//	filter := And(
//	    Eq("typDokl.kod", "FAKTURA"),
//	    Or(
//	        IsNull("datSplat"),
//	        Lt("datSplat", "2026-01-01"),
//	    ),
//	)
//	filter.Build() // → (typDokl.kod = 'FAKTURA' and (datSplat is null or datSplat < '2026-01-01'))
//
// String values are wrapped in single quotes; embedded apostrophes are doubled.
// Numeric values (integers and decimals) are passed unquoted.
//
// # Query builder
//
// Construct a Query and call BuildURL for GET requests or BuildPostBody+PostURL
// for POST /query.json requests (required when the filter expression would
// exceed safe URL lengths, or when the FlexiBee URL limit parameter is ignored).
//
//	q := &Query{
//	    Company:  "demo",
//	    Evidence: "faktura-vydana",
//	    Filter:   Eq("stav", "nevyrizeno"),
//	    Fields:   []string{"id", "kod", "sumCelkem"},
//	    Sort:     []SortField{{Field: "datVyst", Desc: true}},
//	    Limit:    20,
//	    AddCount: true,
//	}
//	path := q.BuildURL()
//	// → /c/demo/faktura-vydana/(stav%20=%20'nevyrizeno').json?detail=custom:id,kod,sumCelkem&order=datVyst@D&limit=20&add-row-count=true
package flexi

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// filterPathEscape encodes a Flexi filter expression for safe embedding as a
// URL path segment.  It percent-encodes bytes that are not permitted in RFC
// 3986 path segments (spaces, '<', '>', '#', '?', '/', and all non-ASCII
// bytes) while deliberately preserving the sub-delimiters that Flexi filter
// syntax requires unencoded: '(', ')', '\”, ',', '=', '+', ';', '!'.
//
// This is a purpose-built alternative to url.PathEscape, which conservatively
// encodes sub-delimiters due to compatibility concerns (golang.org/issue/5765)
// that do not apply here because FlexiBee parses the expression from the
// decoded path.
func filterPathEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isSafeFilterByte(c) {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	return b.String()
}

// isSafeFilterByte reports whether ASCII byte c may appear unencoded inside a
// Flexi filter path segment.  Non-ASCII bytes (≥ 0x80) always require
// encoding; they are handled by the caller.
func isSafeFilterByte(c byte) bool {
	// Unreserved (§2.3)
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
		return true
	}
	switch c {
	case '-', '_', '.', '~': // unreserved non-alphanumeric
		return true
	case '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=': // sub-delims (§2.2 / §3.3)
		return true
	case ':', '@': // pchar extras
		return true
	}
	return false
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

// ── Filter tree ───────────────────────────────────────────────────────────────

// CompareFilter represents a scalar comparison: field op value.
//
// Op must be one of:  "=", "<>", "<", "<=", ">", ">=", "like", "like similar"
// Value is quoted automatically: numeric strings are unquoted; all other
// values are wrapped in single quotes with embedded apostrophes doubled.
type CompareFilter struct {
	Field string
	Op    string
	Value string
}

func (f *CompareFilter) flexi() {}

// Build serialises the comparison as a Flexi filter expression fragment.
func (f *CompareFilter) Build() string {
	return f.Field + " " + f.Op + " " + quoteFilterValue(f.Value)
}

// AndFilter combines children with logical AND.
// The result is wrapped in parentheses to preserve precedence when nested.
type AndFilter struct{ Children []FilterNode }

func (f *AndFilter) flexi() {}

// Build joins all children with " and " and wraps the result in parentheses.
// An empty Children slice produces an empty string.
func (f *AndFilter) Build() string {
	if len(f.Children) == 0 {
		return ""
	}
	parts := make([]string, len(f.Children))
	for i, c := range f.Children {
		parts[i] = c.Build()
	}
	return "(" + strings.Join(parts, " and ") + ")"
}

// OrFilter combines children with logical OR.
// The result is wrapped in parentheses to preserve precedence when nested.
type OrFilter struct{ Children []FilterNode }

func (f *OrFilter) flexi() {}

// Build joins all children with " or " and wraps the result in parentheses.
// An empty Children slice produces an empty string.
func (f *OrFilter) Build() string {
	if len(f.Children) == 0 {
		return ""
	}
	parts := make([]string, len(f.Children))
	for i, c := range f.Children {
		parts[i] = c.Build()
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

// NotFilter negates its single child expression.
type NotFilter struct{ Child FilterNode }

func (f *NotFilter) flexi() {}

// Build produces "not (<child>)".
func (f *NotFilter) Build() string {
	return "not (" + f.Child.Build() + ")"
}

// IsNullFilter tests whether a field value is null.
type IsNullFilter struct{ Field string }

func (f *IsNullFilter) flexi() {}

// Build produces "field is null".
func (f *IsNullFilter) Build() string { return f.Field + " is null" }

// IsEmptyFilter tests whether a field value is empty (zero-length string).
type IsEmptyFilter struct{ Field string }

func (f *IsEmptyFilter) flexi() {}

// Build produces "field is empty".
func (f *IsEmptyFilter) Build() string { return f.Field + " is empty" }

// InFilter tests whether a field value belongs to a set of values.
// Each value is quoted by the same rules as CompareFilter.Value.
type InFilter struct {
	Field  string
	Values []string
}

func (f *InFilter) flexi() {}

// Build produces "field in (v1, v2, ...)".
func (f *InFilter) Build() string {
	parts := make([]string, len(f.Values))
	for i, v := range f.Values {
		parts[i] = quoteFilterValue(v)
	}
	return f.Field + " in (" + strings.Join(parts, ", ") + ")"
}

// ── Query struct ──────────────────────────────────────────────────────────────

// Query describes a bounded, optionally-filtered read request against a
// single FlexiBee evidence.
//
// Use BuildURL for GET requests.
// Use PostURL + BuildPostBody for POST /query.json requests.
type Query struct {
	Evidence string
	Company  string

	// Filter is the root of the typed filter tree; nil means no filter.
	Filter FilterNode

	// Fields is the list of field names to include in each record.
	// Empty means return the default field set.
	Fields []string

	Sort []SortField

	// Limit is the page size.  0 uses the default (50); values > 200 are
	// clamped to the hard cap.
	Limit int

	// Start is the zero-based paging offset.
	Start int

	// AddCount requests add-row-count=true, which causes FlexiBee to return
	// the total count of matching rows in @rowCount.
	AddCount bool
}

// QueryResult captures the resolved context of a Query for use in tool output
// and diagnostics.  It is populated by calling Result() on a Query.
type QueryResult struct {
	Evidence   string // evidence path as used in the URL
	Company    string // company database identifier
	FilterExpr string // resolved Flexi filter expression; empty if no filter
	Limit      int    // effective limit after applying default and hard cap
	Start      int    // paging offset
	AddCount   bool   // whether total row-count was requested
}

// Result resolves the Query to a QueryResult without constructing a URL.
// Callers include the QueryResult in tool output to give AI clients the query
// context alongside the returned records.
func (q *Query) Result() QueryResult {
	expr := ""
	if q.Filter != nil {
		expr = q.Filter.Build()
	}
	return QueryResult{
		Evidence:   q.Evidence,
		Company:    q.Company,
		FilterExpr: expr,
		Limit:      effectiveLimit(q.Limit),
		Start:      q.Start,
		AddCount:   q.AddCount,
	}
}

// BuildURL returns the complete GET URL path (path + query string) for this
// query.
//
// URL structure:
//
//	/c/{company}/{evidence}[/({filter})].json
//	    [?detail=custom:f1,f2,...][&order=f@A,...][&limit=N][&start=M][&add-row-count=true]
//
// The filter expression is embedded as a URL path component using
// filterPathEscape, which preserves RFC 3986 sub-delimiters used by Flexi
// filter syntax ('(', ')', '\”, '=', ',') while encoding spaces, '<', '>',
// and all non-ASCII bytes.  Field names in projections and sort specs are
// escaped with url.PathEscape.
//
// The order parameter uses literal '@' and ',' separators per FlexiBee
// convention (e.g. "order=datVyst@D,kod@A") rather than percent-encoding them,
// since they are structural separators in this parameter's value format.
func (q *Query) BuildURL() string {
	path := "/c/" + q.Company + "/" + q.Evidence
	if q.Filter != nil {
		expr := q.Filter.Build()
		if expr != "" {
			// The filter is a path segment of the form /({expr}).json
			// filterPathEscape preserves ( ) ' = , and encodes space, <, >, etc.
			path += "/(" + filterPathEscape(expr) + ")"
		}
	}
	path += ".json"

	return path + buildQueryString(q, true)
}

// PostURL returns the POST URL path for a /query.json request.
//
// Unlike BuildURL, the filter expression and limit are carried in the request
// body (see BuildPostBody); this URL carries only projection, ordering, and the
// row-count flag as query parameters.  FlexiBee ignores the URL-level limit
// parameter on POST /query.json requests.
func (q *Query) PostURL() string {
	return "/c/" + q.Company + "/" + q.Evidence + "/query.json" +
		buildQueryString(q, false)
}

// BuildPostBody encodes the query as the JSON body for a POST /query.json
// request.
//
// FlexiBee's POST /query.json endpoint reads the filter expression and the
// limit from the winstrom request body; the URL-level limit parameter is
// explicitly ignored by the server.  Projection and ordering are still passed
// as URL query parameters via PostURL.
//
// Body structure:
//
//	{"winstrom": {"@filter": "<expr>", "limit": N, "start": M}}
func (q *Query) BuildPostBody() ([]byte, error) {
	filterExpr := ""
	if q.Filter != nil {
		filterExpr = q.Filter.Build()
	}

	type winstromQuery struct {
		Filter string `json:"@filter,omitempty"`
		Limit  int    `json:"limit"`
		Start  int    `json:"start,omitempty"`
	}
	type envelope struct {
		Winstrom winstromQuery `json:"winstrom"`
	}

	body := envelope{
		Winstrom: winstromQuery{
			Filter: filterExpr,
			Limit:  effectiveLimit(q.Limit),
			Start:  q.Start,
		},
	}
	return json.Marshal(body)
}

// buildQueryString assembles the query-string portion (without "?") for a
// Query.  includeLimitStart controls whether limit and start are included; they
// belong in the URL for GET requests but in the POST body for POST /query.json.
func buildQueryString(q *Query, includeLimitStart bool) string {
	var parts []string

	// detail=custom:f1,f2,...
	// Field names are path-escaped to handle dots, slashes, and Unicode.
	if len(q.Fields) > 0 {
		escaped := make([]string, len(q.Fields))
		for i, f := range q.Fields {
			escaped[i] = url.PathEscape(f)
		}
		parts = append(parts, "detail=custom:"+strings.Join(escaped, ","))
	}

	// order=field@A,field@D
	// The '@' direction separator and ',' list separator are kept literal;
	// they are structural characters in this parameter value, not data that
	// must be percent-encoded for correctness, and FlexiBee expects them
	// without encoding in practice.
	if len(q.Sort) > 0 {
		sortParts := make([]string, len(q.Sort))
		for i, s := range q.Sort {
			dir := "A"
			if s.Desc {
				dir = "D"
			}
			sortParts[i] = url.PathEscape(s.Field) + "@" + dir
		}
		parts = append(parts, "order="+strings.Join(sortParts, ","))
	}

	if includeLimitStart {
		parts = append(parts, "limit="+strconv.Itoa(effectiveLimit(q.Limit)))
		if q.Start > 0 {
			parts = append(parts, "start="+strconv.Itoa(q.Start))
		}
	}

	if q.AddCount {
		parts = append(parts, "add-row-count=true")
	}

	if len(parts) == 0 {
		return ""
	}
	return "?" + strings.Join(parts, "&")
}

// effectiveLimit returns limit clamped to [1, maxLimit], using defaultLimit
// when limit is zero or negative.
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// quoteFilterValue wraps s in single quotes for use in a Flexi filter
// expression, doubling any embedded apostrophes.
//
// Numeric values (valid Go float64 representations) are returned unquoted
// because FlexiBee accepts numeric literals without quotes in filter
// comparisons.
func quoteFilterValue(s string) string {
	if isNumericValue(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// isNumericValue reports whether s is a valid integer or decimal number as
// recognised by strconv.ParseFloat.  Empty strings are not numeric.
func isNumericValue(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// ── Helper constructors ───────────────────────────────────────────────────────

// Eq returns a FilterNode for field = value.
func Eq(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: "=", Value: value}
}

// Ne returns a FilterNode for field <> value.
func Ne(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: "<>", Value: value}
}

// Lt returns a FilterNode for field < value.
func Lt(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: "<", Value: value}
}

// Lte returns a FilterNode for field <= value.
func Lte(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: "<=", Value: value}
}

// Gt returns a FilterNode for field > value.
func Gt(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: ">", Value: value}
}

// Gte returns a FilterNode for field >= value.
func Gte(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: ">=", Value: value}
}

// Like returns a FilterNode for field like value.
func Like(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: "like", Value: value}
}

// LikeSimilar returns a FilterNode for field like similar value.
// "like similar" supports POSIX regular expression matching in Flexi filters.
func LikeSimilar(field, value string) FilterNode {
	return &CompareFilter{Field: field, Op: "like similar", Value: value}
}

// And returns a FilterNode that is the logical AND of all provided children.
// The result is wrapped in parentheses.
func And(children ...FilterNode) FilterNode {
	return &AndFilter{Children: children}
}

// Or returns a FilterNode that is the logical OR of all provided children.
// The result is wrapped in parentheses.
func Or(children ...FilterNode) FilterNode {
	return &OrFilter{Children: children}
}

// Not returns a FilterNode that negates child.
func Not(child FilterNode) FilterNode {
	return &NotFilter{Child: child}
}

// In returns a FilterNode that tests field membership in the provided values.
func In(field string, values ...string) FilterNode {
	return &InFilter{Field: field, Values: values}
}

// IsNull returns a FilterNode that tests whether field is null.
func IsNull(field string) FilterNode {
	return &IsNullFilter{Field: field}
}

// IsEmpty returns a FilterNode that tests whether field is empty.
func IsEmpty(field string) FilterNode {
	return &IsEmptyFilter{Field: field}
}
