package couch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	kivik "github.com/go-kivik/kivik/v4"
)

// SyncDesignDoc creates or updates a design document (or any document) so its
// content matches doc. It is idempotent: unchanged documents are not written.
// doc must contain "_id"; "_rev" is managed automatically.
func SyncDesignDoc(ctx context.Context, store Store, doc map[string]any) (changed bool, err error) {
	id, _ := doc["_id"].(string)
	if id == "" {
		return false, fmt.Errorf("couch: SyncDesignDoc: document has no _id")
	}

	var rev string
	existing, err := store.Get(ctx, id)
	switch {
	case err == nil:
		rev, _ = existing["_rev"].(string)
		if sameContent(existing, doc) {
			return false, nil
		}
	case kivik.HTTPStatus(err) == http.StatusNotFound:
	default:
		return false, err
	}

	body := make(map[string]any, len(doc))
	for k, v := range doc {
		if k != "_id" && k != "_rev" {
			body[k] = v
		}
	}
	if _, _, err := store.Upsert(ctx, id, rev, body); err != nil {
		return false, err
	}
	return true, nil
}

// sameContent compares two documents ignoring _rev, after a JSON round trip so
// numeric types match.
func sameContent(a, b map[string]any) bool {
	norm := func(m map[string]any) any {
		c := make(map[string]any, len(m))
		for k, v := range m {
			if k != "_rev" {
				c[k] = v
			}
		}
		raw, _ := json.Marshal(c)
		var out any
		_ = json.Unmarshal(raw, &out)
		return out
	}
	return reflect.DeepEqual(norm(a), norm(b))
}
