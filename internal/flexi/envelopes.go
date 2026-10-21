// Package flexi provides types and helpers for parsing FlexiBee REST API JSON
// responses.  All struct fields that FlexiBee encodes as JSON strings (including
// boolean flags such as isWritable) are declared as Go strings so that raw
// values are preserved without silent coercion.  Use ParseBoolField to convert
// string booleans when logic requires it.
package flexi

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseBoolField converts a FlexiBee string boolean field to a Go bool.
// Only the exact value "true" maps to true; everything else ("false", "TRUE",
// the empty string, or any unexpected value) maps to false.
func ParseBoolField(s string) bool {
	return s == "true"
}

// ── Company ───────────────────────────────────────────────────────────────────

// Company represents a single FlexiBee company database entry from the
// /c.json response.
type Company struct {
	Version         string `json:"@version"`
	DBNazev         string `json:"dbNazev"`
	Nazev           string `json:"nazev"`
	ICO             string `json:"ico"`
	DIC             string `json:"dic"`
	Email           string `json:"email"`
	InternalVersion string `json:"internalVersion"`
}

// CompaniesEnvelope is the top-level envelope for /c.json responses.
//
//	{"companies": {"company": [...]}}
type CompaniesEnvelope struct {
	Companies struct {
		Company []Company `json:"company"`
	} `json:"companies"`
}

// ── Evidence ──────────────────────────────────────────────────────────────────

// Evidence describes one FlexiBee evidence (entity type) returned by the
// /c/{company}/evidences.json endpoint.
type Evidence struct {
	EvidenceName string `json:"evidenceName"`
	EvidencePath string `json:"evidencePath"`
	// ImportSupported is extracted from the nested importConfig object and
	// carries values such as "SUPPORTED", "NOT_SUPPORTED", or "".
	ImportSupported string
}

// importConfigJSON is an internal helper for decoding the nested importConfig
// sub-object within an Evidence entry.
type importConfigJSON struct {
	ImportSupported string `json:"importSupported"`
}

// evidenceJSON mirrors the on-wire shape of an evidence entry, with the nested
// importConfig preserved for subsequent flattening.
type evidenceJSON struct {
	EvidenceName string           `json:"evidenceName"`
	EvidencePath string           `json:"evidencePath"`
	ImportConfig importConfigJSON `json:"importConfig"`
}

// UnmarshalJSON implements json.Unmarshaler.  It decodes the nested
// importConfig.importSupported value into the flat Evidence.ImportSupported
// field.
func (e *Evidence) UnmarshalJSON(data []byte) error {
	var raw evidenceJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.EvidenceName = raw.EvidenceName
	e.EvidencePath = raw.EvidencePath
	e.ImportSupported = raw.ImportConfig.ImportSupported
	return nil
}

// EvidencesEnvelope is the top-level envelope for
// /c/{company}/evidences.json responses.
//
//	{"evidences": {"evidence": [...]}}
type EvidencesEnvelope struct {
	Evidences struct {
		Evidence []Evidence `json:"evidence"`
	} `json:"evidences"`
}

// ── Property ──────────────────────────────────────────────────────────────────

// Property describes one field (column) of a FlexiBee evidence as returned by
// /c/{company}/{evidence}/properties.json.
//
// IsWritable, IsMandatory, and similar boolean attributes are kept as strings
// because FlexiBee encodes them as JSON strings ("true"/"false").  Use
// ParseBoolField to convert them when needed.
type Property struct {
	Name           string `json:"name"`
	Title          string `json:"title"`
	IsWritable     string `json:"isWritable"`
	IsMandatory    string `json:"isMandatory"`
	Type           string `json:"type"`
	URL            string `json:"url"`
	Notice         string `json:"notice"`
	MaxLength      string `json:"maxLength"`
	FkEvidencePath string `json:"fkEvidencePath"`
	FkEvidenceName string `json:"fkEvidenceName"`
}

// PropertiesEnvelope is the top-level envelope for evidence properties
// responses.
//
//	{"properties": {"property": [...]}}
type PropertiesEnvelope struct {
	Properties struct {
		Property []Property `json:"property"`
	} `json:"properties"`
}

// ── Relation ──────────────────────────────────────────────────────────────────

// Relation describes one related sub-evidence accessible through a parent
// evidence, as returned by /c/{company}/{evidence}/relations.json.
type Relation struct {
	RelationPath string `json:"relationPath"`
	Name         string `json:"name"`
	URL          string `json:"url"`
}

// RelationsEnvelope is the top-level envelope for evidence relation list
// responses.
//
//	{"relations": {"relation": [...]}}
type RelationsEnvelope struct {
	Relations struct {
		Relation []Relation `json:"relation"`
	} `json:"relations"`
}

// ── Report ────────────────────────────────────────────────────────────────────

// Report describes one print report available for a FlexiBee evidence, as
// returned by /c/{company}/{evidence}/reports.json.
//
// IsDefault is a string boolean; use ParseBoolField to convert it.
type Report struct {
	ReportName string `json:"reportName"`
	ReportPath string `json:"reportPath"`
	IsDefault  string `json:"isDefault"`
	Language   string `json:"language"`
}

// ReportsEnvelope is the top-level envelope for evidence report list responses.
//
//	{"reports": {"report": [...]}}
type ReportsEnvelope struct {
	Reports struct {
		Report []Report `json:"report"`
	} `json:"reports"`
}

// ── Action ────────────────────────────────────────────────────────────────────

// Action describes one callable action available on a FlexiBee evidence, as
// returned by /c/{company}/{evidence}/actions.json.
type Action struct {
	ActionName string `json:"actionName"`
	ActionPath string `json:"actionPath"`
	URL        string `json:"url"`
	// NeedInstance is "true" when the action requires a record identifier.
	NeedInstance string `json:"needInstance"`
}

// ActionsEnvelope is the top-level envelope for evidence action list responses.
//
//	{"actions": {"action": [...]}}
type ActionsEnvelope struct {
	Actions struct {
		Action []Action `json:"action"`
	} `json:"actions"`
}

// ── WinstromEnvelope ──────────────────────────────────────────────────────────

// WinstromEnvelope wraps the standard FlexiBee record response.
//
//	{"winstrom": {"@version": "...", "@rowCount": "42", "<evidence>": [...]}}
//
// Because the evidence key is dynamic (e.g. "faktura", "adresar",
// "polozkyFaktury"), the raw JSON array value for every non-@ key is collected
// into Records.  Callers unmarshal Records[0] into the concrete slice type they
// expect:
//
//	var invoices []MyInvoice
//	if len(env.Records) > 0 {
//	    json.Unmarshal(env.Records[0], &invoices)
//	}
//
// Precision of @rowCount and record IDs (which FlexiBee encodes as JSON
// strings) is preserved because both are kept as raw strings or RawMessage
// rather than being coerced to numeric types.
type WinstromEnvelope struct {
	Version  string `json:"@version"`
	RowCount string `json:"@rowCount"`
	// Records holds the raw JSON array(s) for non-@ keys in the winstrom
	// object.  In a typical single-evidence response there is exactly one
	// entry containing the evidence record array.
	Records []json.RawMessage
}

// UnmarshalJSON implements json.Unmarshaler.  It peels the outer "winstrom"
// wrapper, extracts @version and @rowCount, then appends the raw JSON value of
// every non-@ key to Records.
func (w *WinstromEnvelope) UnmarshalJSON(data []byte) error {
	// Step 1 – peel the outer {"winstrom": {...}} wrapper.
	var outer struct {
		Winstrom json.RawMessage `json:"winstrom"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return fmt.Errorf("flexi: parsing winstrom outer envelope: %w", err)
	}
	if len(outer.Winstrom) == 0 {
		return fmt.Errorf("flexi: response has no \"winstrom\" key")
	}

	// Step 2 – parse the inner object into a generic key→raw-value map so
	// that the dynamic evidence key can be handled without reflection.
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(outer.Winstrom, &inner); err != nil {
		return fmt.Errorf("flexi: parsing winstrom inner object: %w", err)
	}

	// Step 3 – extract the well-known @ metadata fields.
	if v, ok := inner["@version"]; ok {
		_ = json.Unmarshal(v, &w.Version)
	}
	if v, ok := inner["@rowCount"]; ok {
		_ = json.Unmarshal(v, &w.RowCount)
	}

	// Step 4 – collect the raw JSON array(s) for non-@ keys (the evidence
	// records).  Order of map iteration is not guaranteed; callers that care
	// must identify the correct Records entry by attempting to unmarshal each.
	for k, v := range inner {
		if strings.HasPrefix(k, "@") {
			continue
		}
		w.Records = append(w.Records, v)
	}
	return nil
}
