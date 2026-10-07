package couch

import (
	"context"
	"fmt"
)

// IndexDefinition defines a Mango index declared in couchgraph.yaml.
type IndexDefinition struct {
	Name   string   `yaml:"name" json:"name"`
	Fields []string `yaml:"fields" json:"fields"`
	DDoc   string   `yaml:"ddoc,omitempty" json:"ddoc,omitempty"`
}

// SyncIndex creates the Mango index if it does not already exist. It returns
// created=true if the index was added, or false if it was already present.
func (r *Repository) SyncIndex(ctx context.Context, idx IndexDefinition) (created bool, err error) {
	if idx.Name == "" {
		return false, fmt.Errorf("couch: index name must not be empty")
	}
	if len(idx.Fields) == 0 {
		return false, fmt.Errorf("couch: index %q must define at least one field", idx.Name)
	}

	indexes, err := r.client.DB().GetIndexes(ctx)
	if err != nil {
		return false, fmt.Errorf("couch: GetIndexes: %w", err)
	}

	for _, existing := range indexes {
		if existing.Name == idx.Name {
			return false, nil
		}
	}

	payload := map[string]any{
		"fields": idx.Fields,
	}
	if err := r.client.DB().CreateIndex(ctx, idx.DDoc, idx.Name, payload); err != nil {
		return false, fmt.Errorf("couch: CreateIndex(%q): %w", idx.Name, err)
	}
	return true, nil
}
