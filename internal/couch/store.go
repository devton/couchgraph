package couch

import "context"

// BulkGetter fetches many documents by ID in one round trip (_bulk_get).
// Results must be index-aligned with ids; missing documents are nil.
type BulkGetter interface {
	BulkGet(ctx context.Context, ids []string) ([]map[string]any, error)
}

// Store is the set of CouchDB operations used by the GraphQL layers (the
// gqlgen resolvers and the dynamic engine). *Repository is the production
// implementation; tests can provide in-memory fakes.
type Store interface {
	BulkGetter

	Get(ctx context.Context, id string) (map[string]any, error)
	Upsert(ctx context.Context, id, rev string, data map[string]any) (newID, newRev string, err error)
	Delete(ctx context.Context, id, rev string) (string, error)
	BulkDocs(ctx context.Context, docs []map[string]any) ([]map[string]any, error)
	Find(ctx context.Context, opts FindOptions) (*FindResult, error)
	QueryView(ctx context.Context, opts ViewOptions) (*ViewResult, error)
	Databases(ctx context.Context) ([]string, error)
	ServerInfo(ctx context.Context) (map[string]any, error)
	SubscribeChanges(ctx context.Context, since string) (<-chan ChangeEvent, error)
}

var _ Store = (*Repository)(nil)
