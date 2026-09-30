package resolver

import "github.com/ton/couchgraph/internal/couch"

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

// Resolver is the root resolver. It holds shared dependencies injected at startup.
type Resolver struct {
	// Repo provides all CouchDB operations.
	Repo *couch.Repository
}
