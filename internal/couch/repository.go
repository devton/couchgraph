package couch

import (
	"context"
	"fmt"

	kivik "github.com/go-kivik/kivik/v4"
	"github.com/google/uuid"
)

// Document is the generic CouchDB document representation used across the
// repository layer. It mirrors the GraphQL Document type.
type Document struct {
	ID  string         `json:"_id"`
	Rev string         `json:"_rev,omitempty"`
	Data map[string]any `json:"-"` // populated manually from raw map
}

// FindOptions mirrors a subset of the CouchDB Mango _find body.
type FindOptions struct {
	Selector map[string]any   `json:"selector"`
	Fields   []string         `json:"fields,omitempty"`
	Sort     []map[string]any `json:"sort,omitempty"`
	Limit    int              `json:"limit,omitempty"`
	Skip     int              `json:"skip,omitempty"`
	Bookmark string           `json:"bookmark,omitempty"`
}

// FindResult mirrors the CouchDB _find response.
type FindResult struct {
	Docs     []map[string]any
	Bookmark string
	Warning  string
}

// ViewOptions represents parameters for a CouchDB view query.
type ViewOptions struct {
	DesignDoc   string
	ViewName    string
	StartKey    any
	EndKey      any
	Limit       int
	Skip        int
	Descending  bool
	IncludeDocs bool
	Reduce      *bool
	GroupLevel  *int
}

// ViewRow is one row returned by a CouchDB view.
type ViewRow struct {
	ID    string
	Key   any
	Value any
	Doc   map[string]any
}

// ViewResult wraps rows plus metadata from a CouchDB view query.
type ViewResult struct {
	Rows      []ViewRow
	TotalRows int
	Offset    int
}

// Repository provides CRUD and query operations against CouchDB.
type Repository struct {
	client *Client
}

// NewRepository constructs a Repository from a connected Client.
func NewRepository(c *Client) *Repository {
	return &Repository{client: c}
}

// newDocID generates a time-ordered UUIDv7 string to use as a CouchDB document ID.
func newDocID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("couch: failed to generate UUIDv7: %w", err)
	}
	return id.String(), nil
}

// ─── Single Document ────────────────────────────────────────────────────────

// Get fetches a single document by ID and returns it as a raw map.
func (r *Repository) Get(ctx context.Context, id string) (map[string]any, error) {
	row := r.client.DB().Get(ctx, id)

	var raw map[string]any
	if err := row.ScanDoc(&raw); err != nil {
		return nil, fmt.Errorf("couch: Get(%q): %w", id, err)
	}
	return raw, nil
}

// Upsert creates or updates a document.
// If id is empty, a UUIDv7 is generated.
// If rev is empty, a new document is created (will fail if id already exists).
func (r *Repository) Upsert(ctx context.Context, id, rev string, data map[string]any) (string, string, error) {
	if id == "" {
		var err error
		id, err = newDocID()
		if err != nil {
			return "", "", err
		}
	}

	doc := make(map[string]any, len(data)+2)
	for k, v := range data {
		doc[k] = v
	}
	doc["_id"] = id
	if rev != "" {
		doc["_rev"] = rev
	}

	newRev, err := r.client.DB().Put(ctx, id, doc)
	if err != nil {
		return "", "", fmt.Errorf("couch: Upsert(%q): %w", id, err)
	}
	return id, newRev, nil
}

// Delete removes a document by ID and revision.
func (r *Repository) Delete(ctx context.Context, id, rev string) (string, error) {
	newRev, err := r.client.DB().Delete(ctx, id, rev)
	if err != nil {
		return "", fmt.Errorf("couch: Delete(%q, %q): %w", id, rev, err)
	}
	return newRev, nil
}

// ─── Bulk Operations ────────────────────────────────────────────────────────

// BulkGet fetches multiple documents by IDs in a single _bulk_get request.
// Missing documents are returned as nil entries (same index as input ids).
func (r *Repository) BulkGet(ctx context.Context, ids []string) ([]map[string]any, error) {
	results := make([]map[string]any, len(ids))

	// Build an index from ID → position in the input slice.
	idx := make(map[string]int, len(ids))
	for i, id := range ids {
		idx[id] = i
	}

	// Kivik v4: BulkGet returns *ResultSet directly (no error return).
	rows := r.client.DB().BulkGet(ctx, docsFromIDs(ids))
	defer rows.Close()

	for rows.Next() {
		var raw map[string]any
		if err := rows.ScanDoc(&raw); err != nil {
			// Document not found or error — leave as nil.
			continue
		}
		id, _ := raw["_id"].(string)
		if pos, ok := idx[id]; ok {
			results[pos] = raw
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("couch: BulkGet rows error: %w", err)
	}

	return results, nil
}

// BulkDocs performs a bulk create/update/delete operation.
func (r *Repository) BulkDocs(ctx context.Context, docs []map[string]any) ([]map[string]any, error) {
	// Assign UUIDv7 IDs to documents that don't have one yet.
	for i, doc := range docs {
		if _, ok := doc["_id"]; !ok {
			id, err := newDocID()
			if err != nil {
				return nil, err
			}
			docs[i]["_id"] = id
		}
	}

	// Kivik v4: BulkDocs accepts []any, so we convert.
	anyDocs := make([]any, len(docs))
	for i, d := range docs {
		anyDocs[i] = d
	}

	bulkResults, err := r.client.DB().BulkDocs(ctx, anyDocs)
	if err != nil {
		return nil, fmt.Errorf("couch: BulkDocs: %w", err)
	}

	out := make([]map[string]any, 0, len(bulkResults))
	for _, res := range bulkResults {
		entry := map[string]any{
			"ok":   res.Error == nil,
			"_id":  res.ID,
			"_rev": res.Rev,
		}
		if res.Error != nil {
			entry["error"] = res.Error.Error()
		}
		out = append(out, entry)
	}
	return out, nil
}

// ─── Mango Queries ──────────────────────────────────────────────────────────

// Find executes a Mango (_find) query.
// Kivik v4: db.Find accepts the query as any and returns *ResultSet.
func (r *Repository) Find(ctx context.Context, opts FindOptions) (*FindResult, error) {
	query := map[string]any{
		"selector": opts.Selector,
	}
	if len(opts.Fields) > 0 {
		query["fields"] = opts.Fields
	}
	if len(opts.Sort) > 0 {
		query["sort"] = opts.Sort
	}
	if opts.Limit > 0 {
		query["limit"] = opts.Limit
	}
	if opts.Skip > 0 {
		query["skip"] = opts.Skip
	}
	if opts.Bookmark != "" {
		query["bookmark"] = opts.Bookmark
	}

	// Kivik v4: Find returns *ResultSet (no error return).
	rows := r.client.DB().Find(ctx, query)
	defer rows.Close()

	var kivikDocs []map[string]any
	for rows.Next() {
		var doc map[string]any
		if err := rows.ScanDoc(&doc); err != nil {
			return nil, fmt.Errorf("couch: Find scan: %w", err)
		}
		kivikDocs = append(kivikDocs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("couch: Find rows: %w", err)
	}

	return &FindResult{
		Docs: kivikDocs,
	}, nil
}

// ─── View Queries ───────────────────────────────────────────────────────────

// QueryView executes a CouchDB MapReduce view.
// Kivik v4: db.Query(ctx, ddoc, view, ...Option) *ResultSet.
func (r *Repository) QueryView(ctx context.Context, opts ViewOptions) (*ViewResult, error) {
	var kopts []kivik.Option

	if opts.StartKey != nil {
		kopts = append(kopts, kivik.Param("startkey", opts.StartKey))
	}
	if opts.EndKey != nil {
		kopts = append(kopts, kivik.Param("endkey", opts.EndKey))
	}
	if opts.Limit > 0 {
		kopts = append(kopts, kivik.Param("limit", opts.Limit))
	}
	if opts.Skip > 0 {
		kopts = append(kopts, kivik.Param("skip", opts.Skip))
	}
	if opts.Descending {
		kopts = append(kopts, kivik.Param("descending", true))
	}
	if opts.IncludeDocs {
		kopts = append(kopts, kivik.Param("include_docs", true))
	}
	if opts.Reduce != nil {
		kopts = append(kopts, kivik.Param("reduce", *opts.Reduce))
	}
	if opts.GroupLevel != nil {
		kopts = append(kopts, kivik.Param("group_level", *opts.GroupLevel))
	}

	// Kivik v4: Query returns *ResultSet directly.
	rows := r.client.DB().Query(ctx, opts.DesignDoc, opts.ViewName, kopts...)
	defer rows.Close()

	var viewRows []ViewRow
	for rows.Next() {
		rowID, _ := rows.ID()
		vr := ViewRow{
			ID: rowID,
		}
		_ = rows.ScanKey(&vr.Key)
		_ = rows.ScanValue(&vr.Value)
		if opts.IncludeDocs {
			var doc map[string]any
			if err := rows.ScanDoc(&doc); err == nil {
				vr.Doc = doc
			}
		}
		viewRows = append(viewRows, vr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("couch: QueryView rows: %w", err)
	}

	return &ViewResult{
		Rows: viewRows,
	}, nil
}

// ─── Server-level Operations ────────────────────────────────────────────────

// Databases lists all database names on the CouchDB server.
func (r *Repository) Databases(ctx context.Context) ([]string, error) {
	dbs, err := r.client.Kivik().AllDBs(ctx)
	if err != nil {
		return nil, fmt.Errorf("couch: Databases: %w", err)
	}
	return dbs, nil
}

// ServerInfo returns version and vendor information from the CouchDB root endpoint.
func (r *Repository) ServerInfo(ctx context.Context) (map[string]any, error) {
	info, err := r.client.Kivik().Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("couch: ServerInfo: %w", err)
	}
	return map[string]any{
		"version": info.Version,
		"vendor":  info.Vendor,
	}, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// docsFromIDs converts a slice of IDs into the format Kivik expects for BulkGet.
func docsFromIDs(ids []string) []kivik.BulkGetReference {
	refs := make([]kivik.BulkGetReference, len(ids))
	for i, id := range ids {
		refs[i] = kivik.BulkGetReference{ID: id}
	}
	return refs
}
