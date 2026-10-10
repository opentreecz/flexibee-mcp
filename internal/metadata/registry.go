// Package metadata provides a bounded TTL cache for FlexiBee schema metadata.
//
// The Registry caches companies, evidence catalogs, properties, relations, and
// reports per company and credential context.  Record data is never cached here
// — only schema metadata whose structure changes rarely.
//
// Cache entries expire after a configurable TTL (default 5 minutes).  Expired
// entries are replaced on next access under a write lock with a double-check to
// avoid redundant upstream calls.  Concurrent reads of fresh entries proceed
// without contention.
//
// Usage:
//
//	reg := metadata.NewRegistry(client, 5*time.Minute)
//	evidences, err := reg.Evidences(ctx, "demo")
//	unknown, err := reg.ValidateFields(ctx, "demo", "faktura-vydana", fields)
package metadata

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/opentreecz/flexibee-mcp/internal/flexi"
)

// defaultTTL is the cache lifetime used when NewRegistry is called with
// ttl <= 0.
const defaultTTL = 5 * time.Minute

// cacheEntry holds a fetched value and the monotonic timestamp of the fetch.
type cacheEntry[T any] struct {
	data      T
	fetchedAt time.Time
}

// isFresh reports whether entry is non-nil and was fetched within ttl.
func isFresh[T any](entry *cacheEntry[T], ttl time.Duration) bool {
	return entry != nil && time.Since(entry.fetchedAt) < ttl
}

// Registry is a bounded, thread-safe metadata cache for a single FlexiBee
// client/credential context.
//
// Map keys are ASCII strings derived from company database identifiers and
// evidence path segments, separated by ":" (company+":"+evidence).
// A Registry must not be copied after first use.
type Registry struct {
	client *flexi.Client
	ttl    time.Duration

	mu         sync.RWMutex
	companies  *cacheEntry[[]flexi.Company]
	evidences  map[string]*cacheEntry[[]flexi.Evidence] // key: company
	properties map[string]*cacheEntry[[]flexi.Property] // key: company+":"+evidence
	relations  map[string]*cacheEntry[[]flexi.Relation] // key: company+":"+evidence
	reports    map[string]*cacheEntry[[]flexi.Report]   // key: company+":"+evidence
}

// NewRegistry constructs a Registry backed by client with the given cache TTL.
// If ttl is zero or negative the defaultTTL (5 minutes) is used.
func NewRegistry(client *flexi.Client, ttl time.Duration) *Registry {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Registry{
		client:     client,
		ttl:        ttl,
		evidences:  make(map[string]*cacheEntry[[]flexi.Evidence]),
		properties: make(map[string]*cacheEntry[[]flexi.Property]),
		relations:  make(map[string]*cacheEntry[[]flexi.Relation]),
		reports:    make(map[string]*cacheEntry[[]flexi.Report]),
	}
}

// ─── Cache accessors ──────────────────────────────────────────────────────────

// Companies returns the list of companies, served from cache when fresh.
// On cache miss it fetches from the client under a write lock, with a
// double-check to avoid duplicate upstream calls.
func (r *Registry) Companies(ctx context.Context) ([]flexi.Company, error) {
	// Fast path: serve from cache without acquiring a write lock.
	r.mu.RLock()
	if isFresh(r.companies, r.ttl) {
		data := r.companies.data
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	// Slow path: acquire write lock and re-check before fetching.
	r.mu.Lock()
	defer r.mu.Unlock()
	if isFresh(r.companies, r.ttl) {
		return r.companies.data, nil
	}

	companies, err := r.client.ListCompanies(ctx)
	if err != nil {
		return nil, fmt.Errorf("metadata: fetching companies: %w", err)
	}
	r.companies = &cacheEntry[[]flexi.Company]{data: companies, fetchedAt: time.Now()}
	return companies, nil
}

// Evidences returns the evidence catalog for company, served from cache when
// fresh.
func (r *Registry) Evidences(ctx context.Context, company string) ([]flexi.Evidence, error) {
	r.mu.RLock()
	if isFresh(r.evidences[company], r.ttl) {
		data := r.evidences[company].data
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if isFresh(r.evidences[company], r.ttl) {
		return r.evidences[company].data, nil
	}

	evidences, err := r.client.ListEvidences(ctx, company)
	if err != nil {
		return nil, fmt.Errorf("metadata: fetching evidences for %q: %w", company, err)
	}
	r.evidences[company] = &cacheEntry[[]flexi.Evidence]{data: evidences, fetchedAt: time.Now()}
	return evidences, nil
}

// Properties returns field definitions for company/evidence, served from cache
// when fresh.
func (r *Registry) Properties(ctx context.Context, company, evidence string) ([]flexi.Property, error) {
	key := company + ":" + evidence

	r.mu.RLock()
	if isFresh(r.properties[key], r.ttl) {
		data := r.properties[key].data
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if isFresh(r.properties[key], r.ttl) {
		return r.properties[key].data, nil
	}

	props, err := r.client.ListProperties(ctx, company, evidence)
	if err != nil {
		return nil, fmt.Errorf("metadata: fetching properties for %q/%q: %w", company, evidence, err)
	}
	r.properties[key] = &cacheEntry[[]flexi.Property]{data: props, fetchedAt: time.Now()}
	return props, nil
}

// Relations returns relation definitions for company/evidence, served from
// cache when fresh.
func (r *Registry) Relations(ctx context.Context, company, evidence string) ([]flexi.Relation, error) {
	key := company + ":" + evidence

	r.mu.RLock()
	if isFresh(r.relations[key], r.ttl) {
		data := r.relations[key].data
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if isFresh(r.relations[key], r.ttl) {
		return r.relations[key].data, nil
	}

	rels, err := r.client.ListRelations(ctx, company, evidence)
	if err != nil {
		return nil, fmt.Errorf("metadata: fetching relations for %q/%q: %w", company, evidence, err)
	}
	r.relations[key] = &cacheEntry[[]flexi.Relation]{data: rels, fetchedAt: time.Now()}
	return rels, nil
}

// Reports returns report definitions for company/evidence, served from cache
// when fresh.
func (r *Registry) Reports(ctx context.Context, company, evidence string) ([]flexi.Report, error) {
	key := company + ":" + evidence

	r.mu.RLock()
	if isFresh(r.reports[key], r.ttl) {
		data := r.reports[key].data
		r.mu.RUnlock()
		return data, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if isFresh(r.reports[key], r.ttl) {
		return r.reports[key].data, nil
	}

	reports, err := r.client.ListReports(ctx, company, evidence)
	if err != nil {
		return nil, fmt.Errorf("metadata: fetching reports for %q/%q: %w", company, evidence, err)
	}
	r.reports[key] = &cacheEntry[[]flexi.Report]{data: reports, fetchedAt: time.Now()}
	return reports, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

// ValidateFields checks field names against the properties metadata for the
// given company/evidence and returns a slice of unrecognised field names.
// An empty return slice means all fields are valid.
// Returns an error only when the metadata fetch itself fails.
func (r *Registry) ValidateFields(ctx context.Context, company, evidence string, fields []string) ([]string, error) {
	props, err := r.Properties(ctx, company, evidence)
	if err != nil {
		return nil, err
	}

	known := make(map[string]struct{}, len(props))
	for _, p := range props {
		known[p.Name] = struct{}{}
	}

	var unknown []string
	for _, f := range fields {
		if _, ok := known[f]; !ok {
			unknown = append(unknown, f)
		}
	}
	return unknown, nil
}

// ValidateRelation checks that the relation path exists in the evidence
// metadata.  Returns a descriptive error if the relation is not found.
//
// The relation argument must be the RelationPath value (e.g. "polozkyDokladu"),
// not the display name, because RelationPath is the URL segment used for
// navigation.
func (r *Registry) ValidateRelation(ctx context.Context, company, evidence, relation string) error {
	rels, err := r.Relations(ctx, company, evidence)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if rel.RelationPath == relation {
			return nil
		}
	}
	return fmt.Errorf(
		"metadata: relation %q not found in evidence %q (company %q); "+
			"use flexi_describe_evidence with section=\"relations\" to list valid relation paths",
		relation, evidence, company,
	)
}

// ─── Cache invalidation ───────────────────────────────────────────────────────

// Refresh clears cached metadata entries.
//
// If company is empty, all cached entries across all companies are cleared.
// Otherwise, only entries for that specific company (evidences, properties,
// relations, reports) are removed.  The shared companies list is never cleared
// by a company-scoped refresh.
func (r *Registry) Refresh(company string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if company == "" {
		// Clear everything including the companies list.
		r.companies = nil
		r.evidences = make(map[string]*cacheEntry[[]flexi.Evidence])
		r.properties = make(map[string]*cacheEntry[[]flexi.Property])
		r.relations = make(map[string]*cacheEntry[[]flexi.Relation])
		r.reports = make(map[string]*cacheEntry[[]flexi.Report])
		return
	}

	// Company-scoped clear: remove the evidence catalog and all per-evidence
	// entries whose key starts with "company:".
	delete(r.evidences, company)
	prefix := company + ":"
	for k := range r.properties {
		if strings.HasPrefix(k, prefix) {
			delete(r.properties, k)
		}
	}
	for k := range r.relations {
		if strings.HasPrefix(k, prefix) {
			delete(r.relations, k)
		}
	}
	for k := range r.reports {
		if strings.HasPrefix(k, prefix) {
			delete(r.reports, k)
		}
	}
}
