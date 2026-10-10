package flexi

import (
	"encoding/json"
	"testing"
)

// ── CompaniesEnvelope ─────────────────────────────────────────────────────────

func TestCompaniesEnvelopeParse(t *testing.T) {
	raw := `{
		"companies": {
			"company": [
				{
					"@version": "1.0",
					"dbNazev": "demo",
					"nazev": "Demo s.r.o.",
					"ico": "12345678",
					"dic": "CZ12345678",
					"email": "info@demo.cz"
				}
			]
		}
	}`

	var env CompaniesEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(env.Companies.Company) != 1 {
		t.Fatalf("got %d companies, want 1", len(env.Companies.Company))
	}
	c := env.Companies.Company[0]
	if c.DBNazev != "demo" {
		t.Errorf("DBNazev = %q, want \"demo\"", c.DBNazev)
	}
	if c.Nazev != "Demo s.r.o." {
		t.Errorf("Nazev = %q, want \"Demo s.r.o.\"", c.Nazev)
	}
	if c.ICO != "12345678" {
		t.Errorf("ICO = %q, want \"12345678\"", c.ICO)
	}
}

func TestCompaniesEnvelopeMultiple(t *testing.T) {
	raw := `{
		"companies": {
			"company": [
				{"dbNazev": "alpha", "nazev": "Alpha"},
				{"dbNazev": "beta",  "nazev": "Beta"}
			]
		}
	}`
	var env CompaniesEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got := len(env.Companies.Company); got != 2 {
		t.Fatalf("got %d companies, want 2", got)
	}
	if env.Companies.Company[1].DBNazev != "beta" {
		t.Errorf("second company DBNazev = %q, want \"beta\"", env.Companies.Company[1].DBNazev)
	}
}

// ── EvidencesEnvelope – ImportSupported flattened from importConfig ───────────

func TestEvidencesEnvelopeImportSupported(t *testing.T) {
	raw := `{
		"evidences": {
			"evidence": [
				{
					"evidenceName": "Faktury vydané",
					"evidencePath": "faktura-vydana",
					"importConfig": {"importSupported": "SUPPORTED"}
				},
				{
					"evidenceName": "Číselník",
					"evidencePath": "ciselnik",
					"importConfig": {"importSupported": "NOT_SUPPORTED"}
				}
			]
		}
	}`

	var env EvidencesEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	evs := env.Evidences.Evidence
	if len(evs) != 2 {
		t.Fatalf("got %d evidences, want 2", len(evs))
	}
	if evs[0].ImportSupported != "SUPPORTED" {
		t.Errorf("ImportSupported[0] = %q, want \"SUPPORTED\"", evs[0].ImportSupported)
	}
	if evs[1].ImportSupported != "NOT_SUPPORTED" {
		t.Errorf("ImportSupported[1] = %q, want \"NOT_SUPPORTED\"", evs[1].ImportSupported)
	}
	if evs[0].EvidencePath != "faktura-vydana" {
		t.Errorf("EvidencePath[0] = %q, want \"faktura-vydana\"", evs[0].EvidencePath)
	}
}

func TestEvidencesEnvelopeImportSupportedMissing(t *testing.T) {
	// When importConfig is absent, ImportSupported must be the zero string.
	raw := `{
		"evidences": {
			"evidence": [
				{"evidenceName": "Something", "evidencePath": "something"}
			]
		}
	}`
	var env EvidencesEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if env.Evidences.Evidence[0].ImportSupported != "" {
		t.Errorf("ImportSupported = %q, want empty string", env.Evidences.Evidence[0].ImportSupported)
	}
}

// ── PropertiesEnvelope – isWritable preserved as string ───────────────────────

func TestPropertiesEnvelopeIsWritableString(t *testing.T) {
	raw := `{
		"properties": {
			"property": [
				{
					"name": "id",
					"title": "ID",
					"isWritable": "false",
					"isMandatory": "false",
					"type": "INTEGER"
				},
				{
					"name": "kod",
					"title": "Kód",
					"isWritable": "true",
					"isMandatory": "true",
					"type": "STRING"
				}
			]
		}
	}`

	var env PropertiesEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	props := env.Properties.Property
	if len(props) != 2 {
		t.Fatalf("got %d properties, want 2", len(props))
	}

	// Raw string values must be preserved exactly.
	if props[0].IsWritable != "false" {
		t.Errorf("IsWritable[0] = %q, want \"false\"", props[0].IsWritable)
	}
	if props[1].IsWritable != "true" {
		t.Errorf("IsWritable[1] = %q, want \"true\"", props[1].IsWritable)
	}
}

func TestPropertiesEnvelopeIsWritableParseBool(t *testing.T) {
	// isWritable="false" must NOT become true via ParseBoolField.
	raw := `{
		"properties": {
			"property": [
				{"name": "id", "isWritable": "false", "isMandatory": "false", "type": "INTEGER"}
			]
		}
	}`
	var env PropertiesEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	p := env.Properties.Property[0]
	if ParseBoolField(p.IsWritable) {
		t.Errorf("ParseBoolField(%q) returned true; expected false", p.IsWritable)
	}
	if ParseBoolField(p.IsMandatory) {
		t.Errorf("ParseBoolField(%q) returned true; expected false", p.IsMandatory)
	}
}

// ── RelationsEnvelope ─────────────────────────────────────────────────────────

func TestRelationsEnvelopeParse(t *testing.T) {
	raw := `{
		"relations": {
			"relation": [
				{
					"relationPath": "polozkyFaktury",
					"name": "Položky faktury",
					"url": "/c/demo/faktura-vydana/1/polozkyFaktury.json"
				}
			]
		}
	}`

	var env RelationsEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(env.Relations.Relation) != 1 {
		t.Fatalf("got %d relations, want 1", len(env.Relations.Relation))
	}
	r := env.Relations.Relation[0]
	if r.RelationPath != "polozkyFaktury" {
		t.Errorf("RelationPath = %q, want \"polozkyFaktury\"", r.RelationPath)
	}
	if r.Name != "Položky faktury" {
		t.Errorf("Name = %q, want \"Položky faktury\"", r.Name)
	}
}

// ── WinstromEnvelope ─────────────────────────────────────────────────────────

func TestWinstromEnvelopeParse(t *testing.T) {
	raw := `{
		"winstrom": {
			"@version": "1.0",
			"@rowCount": "42",
			"faktura-vydana": [{"id": "1", "kod": "F001"}]
		}
	}`

	var env WinstromEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// @version and @rowCount must be decoded.
	if env.Version != "1.0" {
		t.Errorf("Version = %q, want \"1.0\"", env.Version)
	}
	// @rowCount is a string, not a number – large counts must be preserved.
	if env.RowCount != "42" {
		t.Errorf("RowCount = %q, want \"42\"", env.RowCount)
	}
	// The non-@ key "faktura-vydana" must appear as one Records entry.
	if len(env.Records) != 1 {
		t.Fatalf("len(Records) = %d, want 1", len(env.Records))
	}

	// Verify the record array can be further unmarshalled.
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(env.Records[0], &records); err != nil {
		t.Fatalf("unmarshal Records[0] error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
}

func TestWinstromEnvelopeAtPrefixKeysNotInRecords(t *testing.T) {
	// @ keys (@version, @rowCount) must not be appended to Records.
	raw := `{"winstrom": {"@version": "1.0", "@rowCount": "5", "adresar": [{}]}}`
	var env WinstromEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	// Only the non-@ key contributes a Records entry.
	if len(env.Records) != 1 {
		t.Errorf("len(Records) = %d, want 1 (@ keys must be excluded)", len(env.Records))
	}
}

func TestWinstromEnvelopeNoWinstromKey(t *testing.T) {
	// A body without the "winstrom" key must return an error.
	raw := `{"something": {}}`
	var env WinstromEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err == nil {
		t.Error("expected error when winstrom key is absent, got nil")
	}
}

// ── ParseBoolField ────────────────────────────────────────────────────────────

func TestParseBoolField(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"true", true},
		{"false", false},
		{"True", false}, // case-sensitive: "True" ≠ "true"
		{"TRUE", false}, // case-sensitive: "TRUE" ≠ "true"
		{"1", false},
		{"yes", false},
		{"", false},
	}
	for _, tc := range tests {
		got := ParseBoolField(tc.input)
		if got != tc.want {
			t.Errorf("ParseBoolField(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
