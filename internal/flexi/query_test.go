package flexi

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── CompareFilter.Build() ─────────────────────────────────────────────────────

func TestCompareFilterBuildOps(t *testing.T) {
	tests := []struct {
		name string
		node FilterNode
		want string
	}{
		{"eq string", Eq("stav", "nevyrizeno"), "stav = 'nevyrizeno'"},
		{"ne string", Ne("stav", "vyrizeno"), "stav <> 'vyrizeno'"},
		{"lt numeric", Lt("sumCelkem", "1000"), "sumCelkem < 1000"},
		{"lte numeric", Lte("sumCelkem", "500.50"), "sumCelkem <= 500.50"},
		{"gt numeric", Gt("sumCelkem", "0"), "sumCelkem > 0"},
		{"gte numeric", Gte("sumCelkem", "100"), "sumCelkem >= 100"},
		{"like", Like("nazev", "Smith%"), "nazev like 'Smith%'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.node.Build()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── String quoting rules ──────────────────────────────────────────────────────

func TestCompareFilterNumericUnquoted(t *testing.T) {
	// Valid integer and decimal numerics must not be wrapped in quotes.
	tests := []struct {
		value string
		want  string
	}{
		{"42", "f = 42"},
		{"3.14", "f = 3.14"},
		{"-7", "f = -7"},
		{"1e5", "f = 1e5"},
	}
	for _, tc := range tests {
		f := &CompareFilter{Field: "f", Op: "=", Value: tc.value}
		got := f.Build()
		if got != tc.want {
			t.Errorf("value=%q: got %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestCompareFilterStringQuoted(t *testing.T) {
	// Non-numeric strings must be single-quoted.
	f := Eq("typDokl.kod", "FAKTURA")
	got := f.Build()
	want := "typDokl.kod = 'FAKTURA'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCompareFilterApostropheDoubled(t *testing.T) {
	// Embedded apostrophe in value must be doubled: O'Brien → 'O''Brien'
	f := Eq("nazev", "O'Brien")
	got := f.Build()
	want := "nazev = 'O''Brien'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── AndFilter.Build() ────────────────────────────────────────────────────────

func TestAndFilterBuild(t *testing.T) {
	f := And(Eq("a", "x"), Eq("b", "y"))
	got := f.Build()
	want := "(a = 'x' and b = 'y')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAndFilterWrapsChildren(t *testing.T) {
	// Nested Or inside And must remain independently parenthesised.
	f := And(
		Eq("typDokl.kod", "FAKTURA"),
		Or(
			IsNull("datSplat"),
			Lt("datSplat", "2026-01-01"),
		),
	)
	got := f.Build()
	want := "(typDokl.kod = 'FAKTURA' and (datSplat is null or datSplat < '2026-01-01'))"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAndFilterEmpty(t *testing.T) {
	f := &AndFilter{}
	if got := f.Build(); got != "" {
		t.Errorf("empty AndFilter.Build() = %q, want \"\"", got)
	}
}

// ── OrFilter.Build() ─────────────────────────────────────────────────────────

func TestOrFilterBuild(t *testing.T) {
	f := Or(Eq("a", "alpha"), Eq("b", "beta"))
	got := f.Build()
	want := "(a = 'alpha' or b = 'beta')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOrFilterEmpty(t *testing.T) {
	f := &OrFilter{}
	if got := f.Build(); got != "" {
		t.Errorf("empty OrFilter.Build() = %q, want \"\"", got)
	}
}

// ── NotFilter.Build() ────────────────────────────────────────────────────────

func TestNotFilterBuild(t *testing.T) {
	f := Not(Eq("stav", "vyrizeno"))
	got := f.Build()
	want := "not (stav = 'vyrizeno')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── InFilter.Build() ─────────────────────────────────────────────────────────

func TestInFilterStringValues(t *testing.T) {
	f := In("typDokl.kod", "FAKTURA", "ZALOHOVA", "DOBROPIS")
	got := f.Build()
	want := "typDokl.kod in ('FAKTURA', 'ZALOHOVA', 'DOBROPIS')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInFilterNumericValues(t *testing.T) {
	// Numeric values in an In list must not be quoted.
	f := In("id", "1", "2", "3")
	got := f.Build()
	want := "id in (1, 2, 3)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── IsNullFilter.Build() and IsEmptyFilter.Build() ───────────────────────────

func TestIsNullFilterBuild(t *testing.T) {
	f := IsNull("datSplat")
	got := f.Build()
	want := "datSplat is null"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsEmptyFilterBuild(t *testing.T) {
	f := IsEmpty("poznamka")
	got := f.Build()
	want := "poznamka is empty"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── Query.BuildURL() ─────────────────────────────────────────────────────────

func TestQueryBuildURLFull(t *testing.T) {
	q := &Query{
		Company:  "demo",
		Evidence: "faktura-vydana",
		Filter:   Eq("stav", "nevyrizeno"),
		Fields:   []string{"id", "kod", "sumCelkem"},
		Sort:     []SortField{{Field: "datVyst", Desc: true}},
		Limit:    20,
		AddCount: true,
	}
	got := q.BuildURL()

	// Path prefix includes encoded filter expression.
	if !strings.HasPrefix(got, "/c/demo/faktura-vydana/(") {
		t.Errorf("unexpected URL prefix: %q", got)
	}
	// .json separator before query string.
	if !strings.Contains(got, ".json?") {
		t.Errorf("missing .json in URL: %q", got)
	}
	// filter expression appears in the path.
	if !strings.Contains(got, "stav") {
		t.Errorf("filter missing from URL: %q", got)
	}
	// custom field projection.
	if !strings.Contains(got, "detail=custom:id,kod,sumCelkem") {
		t.Errorf("fields missing from URL: %q", got)
	}
	// descending sort.
	if !strings.Contains(got, "order=datVyst@D") {
		t.Errorf("sort missing from URL: %q", got)
	}
	// explicit limit.
	if !strings.Contains(got, "limit=20") {
		t.Errorf("limit missing from URL: %q", got)
	}
	// row-count flag.
	if !strings.Contains(got, "add-row-count=true") {
		t.Errorf("add-row-count missing from URL: %q", got)
	}
}

func TestQueryBuildURLNoFilter(t *testing.T) {
	q := &Query{
		Company:  "demo",
		Evidence: "adresar",
		Limit:    10,
	}
	got := q.BuildURL()
	want := "/c/demo/adresar.json?limit=10"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestQueryBuildURLPaging(t *testing.T) {
	q := &Query{
		Company:  "demo",
		Evidence: "faktura-vydana",
		Limit:    10,
		Start:    20,
	}
	got := q.BuildURL()
	if !strings.Contains(got, "limit=10") {
		t.Errorf("limit missing: %q", got)
	}
	if !strings.Contains(got, "start=20") {
		t.Errorf("start missing: %q", got)
	}
}

func TestQueryBuildURLDefaultLimit(t *testing.T) {
	q := &Query{Company: "demo", Evidence: "adresar"}
	got := q.BuildURL()
	// Zero Limit must apply the default (50).
	if !strings.Contains(got, "limit=50") {
		t.Errorf("default limit missing: %q", got)
	}
}

func TestQueryBuildURLLimitCapped(t *testing.T) {
	q := &Query{Company: "demo", Evidence: "adresar", Limit: 9999}
	got := q.BuildURL()
	if !strings.Contains(got, "limit=200") {
		t.Errorf("limit not clamped to 200: %q", got)
	}
}

func TestQueryBuildURLFilterSpecialChars(t *testing.T) {
	// Diacritics (non-ASCII) and spaces in filter values must be percent-encoded.
	q := &Query{
		Company:  "demo",
		Evidence: "adresar",
		Filter:   Eq("nazev", "Novák"),
	}
	got := q.BuildURL()

	// No raw space may appear anywhere in the URL.
	if strings.Contains(got, " ") {
		t.Errorf("URL contains unencoded space: %q", got)
	}
	// No raw non-ASCII characters (e.g. 'á') may remain unencoded.
	for _, r := range got {
		if r > 0x7F {
			t.Errorf("URL contains non-ASCII rune %q: %s", r, got)
			break
		}
	}
	// The UTF-8 encoding of 'á' (U+00E1) is 0xC3 0xA1 → %C3%A1.
	if !strings.Contains(got, "%C3%A1") {
		t.Errorf("URL does not contain %%C3%%A1 (encoded 'á'): %q", got)
	}
}

func TestQueryBuildURLFilterSlashInValue(t *testing.T) {
	// Slashes in filter values must be percent-encoded so they do not break
	// the URL path parser.
	q := &Query{
		Company:  "demo",
		Evidence: "adresar",
		Filter:   Eq("kod", "A/B"),
	}
	got := q.BuildURL()
	// '/' is not in the safe set; it must be encoded as %2F.
	if !strings.Contains(got, "%2F") {
		t.Errorf("slash not percent-encoded in URL: %q", got)
	}
}

// ── Query.BuildPostBody() ────────────────────────────────────────────────────

func TestQueryBuildPostBody(t *testing.T) {
	q := &Query{
		Company:  "demo",
		Evidence: "faktura-vydana",
		Filter:   Eq("stav", "nevyrizeno"),
		Limit:    25,
		Start:    50,
	}
	body, err := q.BuildPostBody()
	if err != nil {
		t.Fatalf("BuildPostBody error: %v", err)
	}

	var env struct {
		Winstrom struct {
			Filter string `json:"@filter"`
			Limit  int    `json:"limit"`
			Start  int    `json:"start"`
		} `json:"winstrom"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if env.Winstrom.Filter != "stav = 'nevyrizeno'" {
		t.Errorf("@filter = %q, want %q", env.Winstrom.Filter, "stav = 'nevyrizeno'")
	}
	if env.Winstrom.Limit != 25 {
		t.Errorf("limit = %d, want 25", env.Winstrom.Limit)
	}
	if env.Winstrom.Start != 50 {
		t.Errorf("start = %d, want 50", env.Winstrom.Start)
	}
}

func TestQueryBuildPostBodyLimitNotInURL(t *testing.T) {
	// For POST /query.json, limit belongs in the body – PostURL must not carry it.
	q := &Query{
		Company:  "demo",
		Evidence: "faktura-vydana",
		Limit:    25,
	}
	postURL := q.PostURL()
	if strings.Contains(postURL, "limit=") {
		t.Errorf("PostURL should not contain limit parameter: %q", postURL)
	}
}
