// Package metadata – registry_test.go exercises the TTL cache, double-check
// locking, field/relation validation helpers, and cache invalidation.
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/config"
	"github.com/opentreecz/flexibee-mcp/internal/flexi"
)

// ─── Test server ──────────────────────────────────────────────────────────────

// registryServer is a mock FlexiBee server that counts upstream calls per
// endpoint so tests can assert how many times the registry went to the network.
type registryServer struct {
	srv           *httptest.Server
	companyCalls  atomic.Int32
	evidenceCalls atomic.Int32
	propertyCalls atomic.Int32
	relationCalls atomic.Int32
	reportCalls   atomic.Int32
}

func newRegistryServer(t *testing.T) *registryServer {
	t.Helper()
	rs := &registryServer{}

	mux := http.NewServeMux()

	// Companies
	mux.HandleFunc("/c.json", func(w http.ResponseWriter, r *http.Request) {
		rs.companyCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"companies": map[string]interface{}{
				"company": []map[string]interface{}{
					{"dbNazev": "demo", "nazev": "Demo"},
				},
			},
		})
	})

	// Evidence list
	mux.HandleFunc("/c/demo/evidence-list.json", func(w http.ResponseWriter, r *http.Request) {
		rs.evidenceCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"evidences": map[string]interface{}{
				"evidence": []map[string]interface{}{
					{"evidenceName": "Adresář", "evidencePath": "adresar"},
					{"evidenceName": "Faktury vydané", "evidencePath": "faktura-vydana"},
				},
			},
		})
	})

	// Properties for adresar
	mux.HandleFunc("/c/demo/adresar/properties.json", func(w http.ResponseWriter, r *http.Request) {
		rs.propertyCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"properties": map[string]interface{}{
				"property": []map[string]interface{}{
					{"name": "id", "title": "ID", "isWritable": "false"},
					{"name": "nazev", "title": "Název", "isWritable": "true"},
					{"name": "ic", "title": "IČ", "isWritable": "true"},
					{"name": "email", "title": "Email", "isWritable": "true"},
				},
			},
		})
	})

	// Relations for adresar
	mux.HandleFunc("/c/demo/adresar/relations.json", func(w http.ResponseWriter, r *http.Request) {
		rs.relationCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"relations": map[string]interface{}{
				"relation": []map[string]interface{}{
					{"relationPath": "kontakty", "name": "Kontakty"},
					{"relationPath": "smlouvy", "name": "Smlouvy"},
				},
			},
		})
	})

	// Reports for adresar
	mux.HandleFunc("/c/demo/adresar/reports.json", func(w http.ResponseWriter, r *http.Request) {
		rs.reportCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"reports": map[string]interface{}{
				"report": []map[string]interface{}{
					{"reportName": "Etiketa", "reportPath": "etiketa"},
				},
			},
		})
	})

	rs.srv = httptest.NewTLSServer(mux)
	t.Cleanup(rs.srv.Close)
	return rs
}

// newClient builds a flexi.Client pointing at the test server.
func (rs *registryServer) newClient(t *testing.T) *flexi.Client {
	t.Helper()
	cfg := &config.Config{
		BaseURL:       rs.srv.URL,
		Username:      "u",
		Password:      "p",
		TimeoutMS:     5000,
		PageSize:      50,
		AuthMode:      config.AuthModeAPI,
		SkipTLSVerify: true,
	}
	c, err := flexi.NewClient(cfg)
	if err != nil {
		t.Fatalf("flexi.NewClient: %v", err)
	}
	return c
}

// ─── Companies cache ──────────────────────────────────────────────────────────

func TestRegistryCompaniesCacheHit(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	// First call goes to the server.
	_, err := reg.Companies(context.Background())
	if err != nil {
		t.Fatalf("Companies (1st): %v", err)
	}
	if rs.companyCalls.Load() != 1 {
		t.Errorf("companyCalls after 1st call = %d, want 1", rs.companyCalls.Load())
	}

	// Second call must be served from cache without a network round-trip.
	_, err = reg.Companies(context.Background())
	if err != nil {
		t.Fatalf("Companies (2nd): %v", err)
	}
	if rs.companyCalls.Load() != 1 {
		t.Errorf("companyCalls after 2nd call = %d, want 1 (cache hit)", rs.companyCalls.Load())
	}
}

func TestRegistryCompaniesTTLExpiry(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 10*time.Millisecond) // very short TTL

	_, _ = reg.Companies(context.Background())
	time.Sleep(30 * time.Millisecond) // wait for TTL to expire
	_, _ = reg.Companies(context.Background())

	if rs.companyCalls.Load() < 2 {
		t.Errorf("companyCalls = %d, want ≥2 after TTL expiry", rs.companyCalls.Load())
	}
}

// ─── Evidences cache ──────────────────────────────────────────────────────────

func TestRegistryEvidencesCacheHit(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	for i := 0; i < 3; i++ {
		evs, err := reg.Evidences(context.Background(), "demo")
		if err != nil {
			t.Fatalf("Evidences %d: %v", i, err)
		}
		if len(evs) != 2 {
			t.Fatalf("Evidences %d: len = %d, want 2", i, len(evs))
		}
	}
	if n := rs.evidenceCalls.Load(); n != 1 {
		t.Errorf("evidenceCalls = %d, want 1 (all hits cached)", n)
	}
}

func TestRegistryEvidencesPerCompanyIsolation(t *testing.T) {
	// Each company gets its own cache slot; different companies don't collide.
	rs := newRegistryServer(t)

	// Add a handler for company "other".
	mux := rs.srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/c/other/evidence-list.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"evidences":{"evidence":[]}}`)
	})

	reg := NewRegistry(rs.newClient(t), 5*time.Minute)
	_, _ = reg.Evidences(context.Background(), "demo")
	_, _ = reg.Evidences(context.Background(), "other")

	if n := rs.evidenceCalls.Load(); n != 1 {
		t.Errorf("evidenceCalls for demo = %d, want 1", n)
	}
}

// ─── Properties cache ─────────────────────────────────────────────────────────

func TestRegistryPropertiesCacheHit(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	for i := 0; i < 4; i++ {
		_, err := reg.Properties(context.Background(), "demo", "adresar")
		if err != nil {
			t.Fatalf("Properties %d: %v", i, err)
		}
	}
	if n := rs.propertyCalls.Load(); n != 1 {
		t.Errorf("propertyCalls = %d, want 1", n)
	}
}

// ─── Relations cache ──────────────────────────────────────────────────────────

func TestRegistryRelationsCacheHit(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	for i := 0; i < 3; i++ {
		_, err := reg.Relations(context.Background(), "demo", "adresar")
		if err != nil {
			t.Fatalf("Relations %d: %v", i, err)
		}
	}
	if n := rs.relationCalls.Load(); n != 1 {
		t.Errorf("relationCalls = %d, want 1", n)
	}
}

// ─── ValidateFields ───────────────────────────────────────────────────────────

func TestValidateFieldsAllKnown(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	unknown, err := reg.ValidateFields(context.Background(), "demo", "adresar", []string{"id", "nazev", "ic"})
	if err != nil {
		t.Fatalf("ValidateFields: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %v, want empty (all fields known)", unknown)
	}
}

func TestValidateFieldsUnknownFields(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	unknown, err := reg.ValidateFields(context.Background(), "demo", "adresar",
		[]string{"nazev", "noSuchField", "anotherBadField"})
	if err != nil {
		t.Fatalf("ValidateFields: %v", err)
	}
	if len(unknown) != 2 {
		t.Errorf("unknown = %v, want [noSuchField anotherBadField]", unknown)
	}
	for _, u := range unknown {
		if u != "noSuchField" && u != "anotherBadField" {
			t.Errorf("unexpected unknown field %q", u)
		}
	}
}

func TestValidateFieldsEmptyRequestAlwaysOK(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	unknown, err := reg.ValidateFields(context.Background(), "demo", "adresar", nil)
	if err != nil {
		t.Fatalf("ValidateFields(nil): %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %v, want empty for nil field list", unknown)
	}
}

// ─── ValidateRelation ─────────────────────────────────────────────────────────

func TestValidateRelationKnown(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	if err := reg.ValidateRelation(context.Background(), "demo", "adresar", "kontakty"); err != nil {
		t.Errorf("ValidateRelation(kontakty): %v", err)
	}
}

func TestValidateRelationUnknown(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	err := reg.ValidateRelation(context.Background(), "demo", "adresar", "noSuchRelation")
	if err == nil {
		t.Fatal("expected error for unknown relation, got nil")
	}
}

// ─── Refresh ──────────────────────────────────────────────────────────────────

func TestRefreshCompanyScopedClearsEntries(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	// Warm cache.
	_, _ = reg.Evidences(context.Background(), "demo")
	_, _ = reg.Properties(context.Background(), "demo", "adresar")
	if rs.evidenceCalls.Load() != 1 || rs.propertyCalls.Load() != 1 {
		t.Fatal("cache not warmed properly")
	}

	// Refresh for "demo" only.
	reg.Refresh("demo")

	// Re-fetches should go to the network again.
	_, _ = reg.Evidences(context.Background(), "demo")
	_, _ = reg.Properties(context.Background(), "demo", "adresar")
	if rs.evidenceCalls.Load() != 2 {
		t.Errorf("evidenceCalls = %d, want 2 after refresh", rs.evidenceCalls.Load())
	}
	if rs.propertyCalls.Load() != 2 {
		t.Errorf("propertyCalls = %d, want 2 after refresh", rs.propertyCalls.Load())
	}
}

func TestRefreshAllClearsEverything(t *testing.T) {
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	// Warm all caches.
	_, _ = reg.Companies(context.Background())
	_, _ = reg.Evidences(context.Background(), "demo")

	reg.Refresh("") // clear everything

	_, _ = reg.Companies(context.Background())
	_, _ = reg.Evidences(context.Background(), "demo")

	if rs.companyCalls.Load() != 2 {
		t.Errorf("companyCalls = %d, want 2 after full refresh", rs.companyCalls.Load())
	}
	if rs.evidenceCalls.Load() != 2 {
		t.Errorf("evidenceCalls = %d, want 2 after full refresh", rs.evidenceCalls.Load())
	}
}

func TestRefreshGlobalDoesNotClearCompanies(t *testing.T) {
	// Company-scoped Refresh does NOT clear the shared companies list.
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	_, _ = reg.Companies(context.Background())
	reg.Refresh("demo") // scoped to "demo", must not clear the companies list

	_, _ = reg.Companies(context.Background())
	if rs.companyCalls.Load() != 1 {
		t.Errorf("companyCalls = %d, want 1 (company-scoped refresh must not clear companies list)",
			rs.companyCalls.Load())
	}
}

// ─── Concurrent access ────────────────────────────────────────────────────────

func TestRegistryConcurrentAccess(t *testing.T) {
	// Fire multiple goroutines against the same registry; the server should
	// be called exactly once (all but the first goroutine get a cache hit).
	rs := newRegistryServer(t)
	reg := NewRegistry(rs.newClient(t), 5*time.Minute)

	const goroutines = 20
	done := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := reg.Evidences(context.Background(), "demo")
			done <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-done; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	if n := rs.evidenceCalls.Load(); n > 2 {
		// Allow up to 2: one call might overlap before the cache entry is set.
		t.Errorf("evidenceCalls = %d, want ≤2 under concurrent access", n)
	}
}
