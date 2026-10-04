package resolver

import (
	"github.com/ton/couchgraph/internal/couch"
	"github.com/ton/couchgraph/internal/graph/model"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// Resolver is the root resolver. It holds shared dependencies injected at startup.
type Resolver struct {
	// Repo provides all CouchDB operations.
	Repo *couch.Repository
}

// rawToDocument converts a raw CouchDB map into a GraphQL Document model.
func rawToDocument(raw map[string]any) *model.Document {
	if raw == nil {
		return nil
	}
	id, _ := raw["_id"].(string)
	rev, _ := raw["_rev"].(string)

	data := make(map[string]any, len(raw))
	for k, v := range raw {
		if k != "_id" && k != "_rev" {
			data[k] = v
		}
	}

	return &model.Document{
		ID:   id,
		Rev:  rev,
		Data: data,
	}
}
